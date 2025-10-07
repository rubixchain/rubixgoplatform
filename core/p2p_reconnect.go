package core

import (
	"fmt"
	"sync"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// P2PReconnectManager handles reconnection of p2p tunnels after IPFS restarts
type P2PReconnectManager struct {
	log            logger.Logger
	core           *Core
	mu             sync.RWMutex
	activeConns    map[string]*PeerConnInfo // peerID -> connection info
	reconnectDelay time.Duration
}

// PeerConnInfo stores information about an active peer connection
type PeerConnInfo struct {
	PeerID       string
	DID          string
	Peer         *ipfsport.Peer
	LastError    error
	LastErrorAt  time.Time
	ReconnectAt  time.Time
	FailureCount int
}

// NewP2PReconnectManager creates a new p2p reconnect manager
func NewP2PReconnectManager(core *Core) *P2PReconnectManager {
	return &P2PReconnectManager{
		log:            core.log.Named("P2PReconnect"),
		core:           core,
		activeConns:    make(map[string]*PeerConnInfo),
		reconnectDelay: 5 * time.Second,
	}
}

// RegisterConnection registers an active peer connection
func (prm *P2PReconnectManager) RegisterConnection(peerID, did string, peer *ipfsport.Peer) {
	prm.mu.Lock()
	defer prm.mu.Unlock()
	
	prm.activeConns[peerID] = &PeerConnInfo{
		PeerID:       peerID,
		DID:          did,
		Peer:         peer,
		FailureCount: 0,
	}
	
	prm.log.Debug("Registered peer connection", "peerID", peerID)
}

// HandleConnectionError handles a connection error and attempts reconnection
func (prm *P2PReconnectManager) HandleConnectionError(peerID string, err error) (*ipfsport.Peer, error) {
	prm.mu.Lock()
	connInfo, exists := prm.activeConns[peerID]
	if !exists {
		prm.mu.Unlock()
		return nil, fmt.Errorf("no connection info for peer %s", peerID)
	}
	
	// Update error info
	connInfo.LastError = err
	connInfo.LastErrorAt = time.Now()
	connInfo.FailureCount++
	
	// Calculate reconnect delay with exponential backoff
	delay := prm.reconnectDelay * time.Duration(connInfo.FailureCount)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	connInfo.ReconnectAt = time.Now().Add(delay)
	
	prm.mu.Unlock()
	
	prm.log.Info("Connection error detected, scheduling reconnect", 
		"peerID", peerID,
		"error", err,
		"failureCount", connInfo.FailureCount,
		"reconnectDelay", delay)
	
	// Wait for reconnect time
	time.Sleep(delay)
	
	// Attempt to reestablish p2p tunnel
	return prm.reconnectPeer(connInfo)
}

// reconnectPeer attempts to reestablish a p2p connection
func (prm *P2PReconnectManager) reconnectPeer(connInfo *PeerConnInfo) (*ipfsport.Peer, error) {
	prm.log.Info("Attempting to reconnect peer", "peerID", connInfo.PeerID)
	
	// First, close the old connection properly
	if connInfo.Peer != nil {
		connInfo.Peer.Close()
	}
	
	// Wait a bit for cleanup
	time.Sleep(1 * time.Second)
	
	// Try to reestablish connection
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		prm.log.Debug("Reconnection attempt", "peerID", connInfo.PeerID, "attempt", attempt)
		
		// Attempt to open new peer connection
		newPeer, err := prm.core.pm.OpenPeerConn(connInfo.PeerID, connInfo.DID, "rubixgoplatform")
		if err != nil {
			prm.log.Error("Failed to reopen peer connection", 
				"peerID", connInfo.PeerID,
				"attempt", attempt,
				"error", err)
			
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
				continue
			}
			
			// Final attempt failed
			prm.mu.Lock()
			delete(prm.activeConns, connInfo.PeerID)
			prm.mu.Unlock()
			
			return nil, fmt.Errorf("failed to reconnect after %d attempts: %w", maxRetries, err)
		}
		
		// Success - update connection info
		prm.mu.Lock()
		connInfo.Peer = newPeer
		connInfo.FailureCount = 0
		connInfo.LastError = nil
		prm.mu.Unlock()
		
		prm.log.Info("Successfully reconnected to peer", "peerID", connInfo.PeerID)
		return newPeer, nil
	}
	
	return nil, fmt.Errorf("reconnection failed after all attempts")
}

// IsConnectionError checks if an error is a p2p connection error
func (prm *P2PReconnectManager) IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	return contains(errStr, "EOF") ||
		contains(errStr, "connection reset by peer") ||
		contains(errStr, "broken pipe") ||
		contains(errStr, "connection refused") ||
		contains(errStr, "stream reset") ||
		contains(errStr, "use of closed network connection")
}

// GetConnectionStatus returns the status of all connections
func (prm *P2PReconnectManager) GetConnectionStatus() map[string]interface{} {
	prm.mu.RLock()
	defer prm.mu.RUnlock()
	
	status := make(map[string]interface{})
	for peerID, connInfo := range prm.activeConns {
		status[peerID] = map[string]interface{}{
			"failure_count": connInfo.FailureCount,
			"last_error":    connInfo.LastError,
			"last_error_at": connInfo.LastErrorAt,
			"reconnect_at":  connInfo.ReconnectAt,
		}
	}
	
	return status
}

// CleanupConnection removes a connection from tracking
func (prm *P2PReconnectManager) CleanupConnection(peerID string) {
	prm.mu.Lock()
	defer prm.mu.Unlock()
	
	if connInfo, exists := prm.activeConns[peerID]; exists {
		if connInfo.Peer != nil {
			connInfo.Peer.Close()
		}
		delete(prm.activeConns, peerID)
		prm.log.Debug("Cleaned up connection", "peerID", peerID)
	}
}