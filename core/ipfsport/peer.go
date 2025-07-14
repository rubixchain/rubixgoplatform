package ipfsport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path"
	"time"

	"sync"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	srvcfg "github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// Port request structure for queuing
type PortRequest struct {
	ID       string
	Priority int // Higher number = higher priority
	Chan     chan uint16
	Timeout  time.Duration
}

// Peer handle for all peer connection
type PeerManager struct {
	peerID    string
	lock      sync.Mutex
	ps        []bool
	appName   string
	ipfs      *ipfsnode.Shell
	log       logger.Logger
	startPort uint16
	lport     uint16
	bootStrap []string

	// Port queuing system
	portQueue    []*PortRequest
	queueLock    sync.Mutex
	queueRunning bool
	stopQueue    chan struct{}

	// Active connection tracking
	activeConnections map[uint16]*Peer
	connLock          sync.RWMutex
}

type Peer struct {
	ensweb.Client
	port     uint16
	local    bool
	log      logger.Logger
	pm       *PeerManager
	peerID   string
	did      string
	protocol string // Store the protocol used for this connection
}

func (peer Peer) GetPeerID() string {
	return peer.peerID
}

func NewPeerManager(startPort uint16, lport uint16, maxNumPort uint16, ipfs *ipfsnode.Shell, log logger.Logger, bootStrap []string, peerID string) *PeerManager {
	p := &PeerManager{
		peerID:            peerID,
		ipfs:              ipfs,
		log:               log.Named("PeerManager"),
		ps:                make([]bool, maxNumPort),
		startPort:         startPort,
		lport:             lport,
		bootStrap:         bootStrap,
		portQueue:         make([]*PortRequest, 0),
		stopQueue:         make(chan struct{}),
		activeConnections: make(map[uint16]*Peer),
	}

	// Start port queue processor
	p.startPortQueue()

	for _, bs := range p.bootStrap {
		_, bsID := path.Split(bs)
		err := p.ipfs.SwarmConnect(context.Background(), "/ipfs/"+bsID)
		if err == nil {
			p.log.Info(fmt.Sprintf("Bootstrap swarm %v connected", bsID))
		} else {
			p.log.Error(fmt.Sprintf("Bootstrap swarm %v failed to connect, err: %v", bsID, err))
		}
	}
	return p
}

// startPortQueue starts the port queue processor
func (pm *PeerManager) startPortQueue() {
	if pm.queueRunning {
		return
	}
	pm.queueRunning = true
	go pm.processPortQueue()
}

// StopPortQueue stops the port queue processor
func (pm *PeerManager) StopPortQueue() {
	if !pm.queueRunning {
		return
	}
	close(pm.stopQueue)
	pm.queueRunning = false
}

// processPortQueue processes port requests in priority order
func (pm *PeerManager) processPortQueue() {
	ticker := time.NewTicker(100 * time.Millisecond)
	cleanupTicker := time.NewTicker(30 * time.Second) // Periodic cleanup
	defer ticker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-pm.stopQueue:
			return
		case <-ticker.C:
			pm.queueLock.Lock()
			if len(pm.portQueue) == 0 {
				pm.queueLock.Unlock()
				continue
			}

			// Sort by priority (highest first)
			for i := 0; i < len(pm.portQueue)-1; i++ {
				for j := i + 1; j < len(pm.portQueue); j++ {
					if pm.portQueue[i].Priority < pm.portQueue[j].Priority {
						pm.portQueue[i], pm.portQueue[j] = pm.portQueue[j], pm.portQueue[i]
					}
				}
			}

			// Try to fulfill requests
			remaining := make([]*PortRequest, 0)
			for _, req := range pm.portQueue {
				port := pm.tryGetPort()
				if port != 0 {
					select {
					case req.Chan <- port:
						pm.log.Debug("Port allocated from queue", "requestID", req.ID, "port", port)
					case <-time.After(100 * time.Millisecond):
						pm.releasePeerPort(port)
						remaining = append(remaining, req)
					}
				} else {
					remaining = append(remaining, req)
				}
			}
			pm.portQueue = remaining
			pm.queueLock.Unlock()
		case <-cleanupTicker.C:
			// Periodic cleanup of stuck ports
			pm.ForceReleaseStuckPorts()
		}
	}
}

// tryGetPort attempts to get a port without queuing
func (pm *PeerManager) tryGetPort() uint16 {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	// First, try to find any marked as available
	for i, status := range pm.ps {
		if !status {
			port := pm.startPort + uint16(i)
			if isPortAvailableWithTimeout(port, 100*time.Millisecond) {
				pm.ps[i] = true
				pm.log.Debug("Port allocated", "port", port, "index", i)
				return port
			} else {
				// Mark as used if port is not actually available
				pm.ps[i] = true
				pm.log.Debug("Port marked as used (not available)", "port", port, "index", i)
			}
		}
	}

	// If we get here, all ports are marked as used
	// Let's do a cleanup pass to check if any ports are actually available
	pm.log.Debug("All ports marked as used, performing cleanup check")
	for i, status := range pm.ps {
		if status {
			port := pm.startPort + uint16(i)
			if isPortAvailableWithTimeout(port, 100*time.Millisecond) {
				pm.log.Debug("Found available port during cleanup", "port", port, "index", i)
				return port
			}
		}
	}

	return 0
}

// requestPort queues a port request with priority
func (pm *PeerManager) requestPort(requestID string, priority int, timeout time.Duration) (uint16, error) {
	// First try to get port immediately
	port := pm.tryGetPort()
	if port != 0 {
		return port, nil
	}

	// If no port available, queue the request
	req := &PortRequest{
		ID:       requestID,
		Priority: priority,
		Chan:     make(chan uint16, 1),
		Timeout:  timeout,
	}

	pm.queueLock.Lock()
	pm.portQueue = append(pm.portQueue, req)
	pm.queueLock.Unlock()

	pm.log.Debug("Port request queued", "requestID", requestID, "priority", priority, "queueLength", len(pm.portQueue))

	// Wait for port allocation or timeout
	select {
	case port := <-req.Chan:
		return port, nil
	case <-time.After(timeout):
		// Remove from queue on timeout
		pm.queueLock.Lock()
		for i, qreq := range pm.portQueue {
			if qreq.ID == requestID {
				pm.portQueue = append(pm.portQueue[:i], pm.portQueue[i+1:]...)
				break
			}
		}
		pm.queueLock.Unlock()
		return 0, fmt.Errorf("port request timeout after %v", timeout)
	}
}

// getPeerPortWithPriority gets a port with specified priority (higher number = higher priority)
func (pm *PeerManager) getPeerPortWithPriority(requestID string, priority int, timeout time.Duration) (uint16, error) {
	return pm.requestPort(requestID, priority, timeout)
}

// GetPortUsageStats returns statistics about port usage
func (pm *PeerManager) GetPortUsageStats() map[string]interface{} {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	totalPorts := len(pm.ps)
	usedPorts := 0
	availablePorts := 0

	for i, status := range pm.ps {
		if status {
			usedPorts++
		} else {
			port := pm.startPort + uint16(i)
			if isPortAvailableWithTimeout(port, 50*time.Millisecond) {
				availablePorts++
			}
		}
	}

	pm.queueLock.Lock()
	queueLength := len(pm.portQueue)
	pm.queueLock.Unlock()

	pm.connLock.RLock()
	activeConnCount := len(pm.activeConnections)
	pm.connLock.RUnlock()

	return map[string]interface{}{
		"totalPorts":        totalPorts,
		"usedPorts":         usedPorts,
		"availablePorts":    availablePorts,
		"startPort":         pm.startPort,
		"endPort":           pm.startPort + uint16(totalPorts-1),
		"queueLength":       queueLength,
		"activeConnections": activeConnCount,
	}
}

// registerActiveConnection registers an active peer connection
func (pm *PeerManager) registerActiveConnection(port uint16, peer *Peer) {
	pm.connLock.Lock()
	defer pm.connLock.Unlock()
	pm.activeConnections[port] = peer
	pm.log.Debug("Registered active connection", "port", port, "peerID", peer.peerID)
}

// unregisterActiveConnection unregisters an active peer connection
func (pm *PeerManager) unregisterActiveConnection(port uint16) {
	pm.connLock.Lock()
	defer pm.connLock.Unlock()
	if _, exists := pm.activeConnections[port]; exists {
		delete(pm.activeConnections, port)
		pm.log.Debug("Unregistered active connection", "port", port)
	}
}

// getActiveConnectionCount returns the number of active connections
func (pm *PeerManager) getActiveConnectionCount() int {
	pm.connLock.RLock()
	defer pm.connLock.RUnlock()
	return len(pm.activeConnections)
}

// isPortActivelyUsed checks if a port is actively being used by a connection
func (pm *PeerManager) isPortActivelyUsed(port uint16) bool {
	pm.connLock.RLock()
	defer pm.connLock.RUnlock()
	_, exists := pm.activeConnections[port]
	return exists
}

func isPortAvailable(port uint16) bool {
	// Convert uint16 port to int
	portInt := int(port)

	// Attempt to listen on the port
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", portInt))
	if err != nil {
		// Port is not available
		return false
	}
	defer listener.Close()

	// Port is available
	return true
}

// isPortAvailableWithTimeout checks port availability with a timeout
func isPortAvailableWithTimeout(port uint16, timeout time.Duration) bool {
	done := make(chan bool, 1)
	go func() {
		done <- isPortAvailable(port)
	}()

	select {
	case result := <-done:
		return result
	case <-time.After(timeout):
		return false
	}
}

func (pm *PeerManager) releasePeerPort(port uint16) bool {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	offset := uint16(port) - pm.startPort
	if int(offset) >= len(pm.ps) {
		pm.log.Warn("Attempted to release port outside range", "port", port, "startPort", pm.startPort)
		return false
	}
	if !pm.ps[offset] {
		pm.log.Debug("Port already released", "port", port, "index", offset)
		return true
	}
	pm.ps[offset] = false
	pm.log.Debug("Port released", "port", port, "index", offset)
	return true
}

func (pm *PeerManager) SwarmConnect(peerID string) bool {
	err := pm.ipfs.SwarmConnect(context.Background(), "/ipfs/"+peerID)
	if err == nil {
		return true
	}
	for _, bs := range pm.bootStrap {
		_, bsID := path.Split(bs)
		pm.log.Debug(bsID)
		err := pm.ipfs.SwarmConnect(context.Background(), "/ipfs/"+bsID)
		if err != nil {
			pm.log.Error("failed to connect bootstrap peer", "BootStrap", bsID, "err", err)
			continue
		}
		err = pm.ipfs.SwarmConnect(context.Background(), "/ipfs/"+bsID+"/p2p-circuit/ipfs/"+peerID)
		if err == nil {
			return true
		} else {
			pm.log.Error("failed to connect peer", "BootStrap", bsID, "err", err)
		}
	}
	return false
}

// ForceReleaseAllPorts releases all ports (for emergency situations)
func (pm *PeerManager) ForceReleaseAllPorts() {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	releasedCount := 0
	for i := range pm.ps {
		if pm.ps[i] {
			releasedCount++
		}
		pm.ps[i] = false
	}
	pm.log.Warn("Force released all ports", "releasedCount", releasedCount)
}

// SafeForceReleaseAllPorts releases only ports that are actually available (safer for bulk transfers)
func (pm *PeerManager) SafeForceReleaseAllPorts() {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	releasedCount := 0
	for i, status := range pm.ps {
		if status {
			port := pm.startPort + uint16(i)
			// Only release if port is actually available (not actively used)
			if isPortAvailableWithTimeout(port, 50*time.Millisecond) {
				pm.ps[i] = false
				releasedCount++
				pm.log.Debug("Safe force released port", "port", port, "index", i)
			} else {
				pm.log.Debug("Port still in use, not releasing", "port", port, "index", i)
			}
		}
	}
	pm.log.Warn("Safe force released ports", "releasedCount", releasedCount)
}

// ForceReleaseStuckPorts releases ports that are marked as used but are actually available
func (pm *PeerManager) ForceReleaseStuckPorts() {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	releasedCount := 0
	for i, status := range pm.ps {
		if status {
			port := pm.startPort + uint16(i)
			if isPortAvailableWithTimeout(port, 50*time.Millisecond) {
				pm.ps[i] = false
				releasedCount++
				pm.log.Debug("Force released stuck port", "port", port, "index", i)
			}
		}
	}
	pm.log.Info("Force released stuck ports", "releasedCount", releasedCount)
}

func (pm *PeerManager) OpenPeerConn(peerID string, did string, appname string) (*Peer, error) {
	// local peer
	if peerID == pm.peerID {
		var err error
		scfg := &srvcfg.Config{
			ServerAddress: "localhost",
			ServerPort:    fmt.Sprintf("%d", pm.lport),
		}
		client, err := ensweb.NewClient(scfg, pm.log)
		if err != nil {
			pm.log.Error("failed to create ensweb client", "err", err)
			return nil, err
		}
		p := &Peer{
			Client: client,
			port:   pm.lport,
			local:  true,
			log:    pm.log,
			pm:     pm,
			peerID: peerID,
			did:    did,
		}
		return p, err
	}
	// remote peer
	if !pm.SwarmConnect(peerID) {
		return nil, fmt.Errorf("failed to connect swarm peer")
	}
	portNum, err := pm.getPeerPortWithPriority("default", 1, 30*time.Second)
	if err != nil {
		pm.log.Error("Failed to get port from queue", "err", err)
		return nil, err
	}
	if portNum == 0 {
		// Log port usage stats when we can't get a port
		stats := pm.GetPortUsageStats()
		pm.log.Error("All ports are busy - cannot create peer connection",
			"peerID", peerID,
			"portStats", stats)
		return nil, fmt.Errorf("all ports are busy")
	}
	// Set up IPFS port forwarding to the remote peer
	addr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", portNum)
	target := fmt.Sprintf("/p2p/%s", peerID)
	pm.log.Debug("Setting up IPFS port forwarding", "addr", addr, "peerID", peerID, "port", portNum)

	// Use the working syntax with positional arguments and dynamic protocol
	proto := "/x/" + appname + "/1.0"
	resp, err := pm.ipfs.Request("p2p/forward", proto, addr, target).Send(context.Background())
	if err != nil {
		pm.log.Error("failed to setup ipfs port forwarding", "err", err, "addr", addr, "peerID", peerID)
		pm.releasePeerPort(portNum)
		return nil, err
	}
	defer resp.Close()
	if resp.Error != nil {
		pm.log.Error("failed to setup ipfs port forwarding", "err", resp.Error, "addr", addr, "peerID", peerID)
		pm.releasePeerPort(portNum)
		return nil, fmt.Errorf("failed to setup ipfs port forwarding: %v", resp.Error)
	}
	pm.log.Debug("Successfully set up IPFS port forwarding", "addr", addr, "peerID", peerID, "port", portNum)

	scfg := &srvcfg.Config{
		ServerAddress: "localhost",
		ServerPort:    fmt.Sprintf("%d", portNum),
	}
	client, err := ensweb.NewClient(scfg, pm.log)
	if err != nil {
		pm.log.Error("failed to create ensweb client", "err", err)
		pm.releasePeerPort(portNum)
		return nil, err
	}
	p := &Peer{
		Client:   client,
		port:     portNum,
		local:    false,
		log:      pm.log,
		pm:       pm,
		peerID:   peerID,
		did:      did,
		protocol: proto, // Store the protocol used for this connection
	}

	// Register the active connection
	pm.registerActiveConnection(portNum, p)

	return p, nil
}

// OpenPeerConnWithPriority opens a peer connection with specified priority for port allocation
func (pm *PeerManager) OpenPeerConnWithPriority(peerID string, did string, appname string, requestID string, priority int, timeout time.Duration) (*Peer, error) {
	// local peer
	if peerID == pm.peerID {
		var err error
		scfg := &srvcfg.Config{
			ServerAddress: "localhost",
			ServerPort:    fmt.Sprintf("%d", pm.lport),
		}
		client, err := ensweb.NewClient(scfg, pm.log)
		if err != nil {
			pm.log.Error("failed to create ensweb client", "err", err)
			return nil, err
		}
		p := &Peer{
			Client: client,
			port:   pm.lport,
			local:  true,
			log:    pm.log,
			pm:     pm,
			peerID: peerID,
			did:    did,
		}
		return p, err
	}

	// remote peer
	if !pm.SwarmConnect(peerID) {
		return nil, fmt.Errorf("failed to connect swarm peer")
	}

	portNum, err := pm.getPeerPortWithPriority(requestID, priority, timeout)
	if err != nil {
		pm.log.Error("Failed to get port from queue", "err", err)
		return nil, err
	}
	if portNum == 0 {
		// Log port usage stats when we can't get a port
		stats := pm.GetPortUsageStats()
		pm.log.Error("All ports are busy - cannot create peer connection",
			"peerID", peerID,
			"portStats", stats)
		return nil, fmt.Errorf("all ports are busy")
	}
	// Set up IPFS port forwarding to the remote peer
	addr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", portNum)
	target := fmt.Sprintf("/p2p/%s", peerID)
	pm.log.Debug("Setting up IPFS port forwarding", "addr", addr, "peerID", peerID, "port", portNum)

	// Use the working syntax with positional arguments and dynamic protocol
	proto := "/x/" + appname + "/1.0"
	resp, err := pm.ipfs.Request("p2p/forward", proto, addr, target).Send(context.Background())
	if err != nil {
		pm.log.Error("failed to setup ipfs port forwarding", "err", err, "addr", addr, "peerID", peerID)
		pm.releasePeerPort(portNum)
		return nil, err
	}
	defer resp.Close()
	if resp.Error != nil {
		pm.log.Error("failed to setup ipfs port forwarding", "err", resp.Error, "addr", addr, "peerID", peerID)
		pm.releasePeerPort(portNum)
		return nil, fmt.Errorf("failed to setup ipfs port forwarding: %v", resp.Error)
	}
	pm.log.Debug("Successfully set up IPFS port forwarding", "addr", addr, "peerID", peerID, "port", portNum)

	scfg := &srvcfg.Config{
		ServerAddress: "localhost",
		ServerPort:    fmt.Sprintf("%d", portNum),
	}
	client, err := ensweb.NewClient(scfg, pm.log)
	if err != nil {
		pm.log.Error("failed to create ensweb client", "err", err)
		pm.releasePeerPort(portNum)
		return nil, err
	}

	p := &Peer{
		Client:   client,
		port:     portNum,
		local:    false,
		log:      pm.log,
		pm:       pm,
		peerID:   peerID,
		did:      did,
		protocol: proto, // Store the protocol used for this connection
	}

	// Register the active connection
	pm.registerActiveConnection(portNum, p)

	return p, nil
}

func (p *Peer) SendJSONRequest(method string, path string, querry map[string]string, req interface{}, resp interface{}, did bool, timeout ...time.Duration) error {
	var err error
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		httpReq, err := p.JSONRequest(method, path, req)
		if err != nil {
			p.log.Error("failed to create request", "attempt", attempt, "err", err)
			continue
		}
		httpReq.Close = true
		if did {
			q := httpReq.URL.Query()
			q.Add("did", p.did)
			httpReq.URL.RawQuery = q.Encode()
		}
		for k, v := range querry {
			q := httpReq.URL.Query()
			q.Add(k, v)
			httpReq.URL.RawQuery = q.Encode()
		}
		httpResp, err := p.Do(httpReq, timeout...)
		if err != nil {
			p.log.Error("failed to receive reply", "attempt", attempt, "err", err)
			time.Sleep(time.Second * time.Duration(attempt)) // Exponential backoff
			continue
		}
		defer httpResp.Body.Close()
		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode == http.StatusInternalServerError {
				err = fmt.Errorf("failed to get tokenchain, tokenchain does not exist in records")
				p.log.Error("TC not available to sync. Possibly DS", "err", err)
				//time.Sleep(time.Second * time.Duration(attempt)) // Exponential backoff
				return err
			}
			err = fmt.Errorf("failed with status code %d", httpResp.StatusCode)
			p.log.Error("request failed", "attempt", attempt, "status", httpResp.StatusCode)
			time.Sleep(time.Second * time.Duration(attempt)) // Exponential backoff
			continue
		}

		if resp != nil {
			err = jsonutil.DecodeJSONFromReader(httpResp.Body, resp)
			if err != nil {
				p.log.Error("invalid response", "attempt", attempt, "err", err)
				time.Sleep(time.Second * time.Duration(attempt)) // Exponential backoff
				continue
			}
		}
		// If we reach here, the request was successful
		return nil
	}
	// Return the last error encountered
	return err
}

func (p *Peer) IsLocal() bool {
	return p.local
}

// UpdateIPFS updates the IPFS shell reference in the peer manager
func (pm *PeerManager) UpdateIPFS(newShell *ipfsnode.Shell) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.ipfs = newShell
}

func (p *Peer) Close() error {
	if !p.local {
		// Unregister the active connection first
		p.pm.unregisterActiveConnection(p.port)

		// Always release the port, regardless of IPFS close success/failure
		defer p.pm.releasePeerPort(p.port)

		// Close the IPFS port forwarding
		addr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", p.port)
		proto := p.protocol
		p.log.Debug("Closing IPFS port forwarding", "proto", proto, "addr", addr)
		resp, err := p.pm.ipfs.Request("p2p/close", proto, addr).Send(context.Background())
		if err != nil {
			p.log.Error("failed to close ipfs port forwarding", "err", err)
			// Don't return error here - we still want to release the port
			return nil
		}
		defer resp.Close()
		if resp.Error != nil {
			p.log.Error("failed to close ipfs port forwarding", "err", resp.Error)
			// Don't return error here - we still want to release the port
			return nil
		}
		p.log.Debug("Closed IPFS port forwarding", "port", p.port, "peerID", p.peerID)
	}
	return nil
}

func (p *Peer) GetPeerDID() string {
	return p.did
}
