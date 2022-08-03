package ipfsport

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	srvcfg "github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/EnsurityTechnologies/logger"
	ipfsnode "github.com/ipfs/go-ipfs-api"
)

// Peer handle for all peer connection
type PeerManager struct {
	lock      sync.Mutex
	ps        []bool
	appName   string
	ipfs      *ipfsnode.Shell
	log       logger.Logger
	startPort uint16
}

type Peer struct {
	ensweb.Client
	port uint16
	log  logger.Logger
	pm   *PeerManager
}

func NewPeerManager(startPort uint16, maxNumPort uint16, ipfs *ipfsnode.Shell, log logger.Logger) *PeerManager {
	p := &PeerManager{
		ipfs:      ipfs,
		log:       log.Named("PeerManager"),
		ps:        make([]bool, maxNumPort),
		startPort: startPort,
	}
	return p
}

func (pm *PeerManager) getPeerPort() uint16 {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for i, status := range pm.ps {
		if !status {
			pm.ps[i] = true
			return pm.startPort + uint16(i)
		}
	}
	return 0
}

func (pm *PeerManager) releasePeerPort(port uint16) bool {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	offset := pm.startPort - uint16(port)
	if int(offset) >= len(pm.ps) {
		return false
	}
	pm.ps[offset] = false
	return false
}

func (pm *PeerManager) OpenPeerConn(peerdID string, appname string) (*Peer, error) {
	err := pm.ipfs.SwarmConnect(context.Background(), "/ipfs/"+peerdID)
	if err != nil {
		pm.log.Error("Failed to connect swarm peer", "err", err)
		return nil, err
	}
	portNum := pm.getPeerPort()
	if portNum == 0 {
		return nil, fmt.Errorf("all ports are busy")
	}
	scfg := &srvcfg.Config{
		ServerAddress: "localhost",
		ServerPort:    fmt.Sprintf("%d", portNum),
	}
	p := &Peer{
		port: portNum,
		pm:   pm,
		log:  pm.log.Named(peerdID),
	}
	proto := "/x/" + appname + "/1.0"
	addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", portNum)
	peer := "/p2p/" + peerdID
	resp, err := pm.ipfs.Request("p2p/forward", proto, addr, peer).Send(context.Background())
	if err != nil {
		pm.log.Error("failed make forward request")
		pm.releasePeerPort(portNum)
		return nil, err
	}
	if resp.Error != nil {
		pm.log.Error("error in forward request")
		pm.releasePeerPort(portNum)
		return nil, resp.Error
	}
	p.Client, err = ensweb.NewClient(scfg, p.log)
	if err != nil {
		pm.log.Error("failed to create ensweb clent", "err", err)
		pm.releasePeerPort(portNum)
		return nil, err
	}
	return p, nil
}

func (p *Peer) SendJSONRequest(method string, path string, req interface{}, resp interface{}, timeout ...time.Duration) error {
	httpReq, err := p.JSONRequest(method, path, req)
	if err != nil {
		return err
	}
	httpResp, err := p.Do(httpReq, timeout...)
	if err != nil {
		p.log.Error("failed to receive reply")
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed with status code %d", httpResp.StatusCode)
	}
	if resp != nil {
		err = jsonutil.DecodeJSONFromReader(httpResp.Body, resp)
		if err != nil {
			return fmt.Errorf("invalid response")
		}
	}
	return nil
}

func (p *Peer) Close() error {
	addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", p.port)
	req := p.pm.ipfs.Request("p2p/close")
	resp, err := req.Option("listen-address", addr).Send(context.Background())
	if err != nil {
		p.log.Error("failed to close ipfs port", "err", err)
		return err
	}
	if resp.Error != nil {
		p.log.Error("failed to close ipfs port", "err", resp.Error)
		return resp.Error
	}
	p.pm.releasePeerPort(p.port)
	return nil
}
