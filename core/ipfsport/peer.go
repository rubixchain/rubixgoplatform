package ipfsport

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	srvcfg "github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

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
}

type Peer struct {
	ensweb.Client
	port   uint16
	local  bool
	log    logger.Logger
	pm     *PeerManager
	peerID string
	did    string
}

func NewPeerManager(startPort uint16, lport uint16, maxNumPort uint16, ipfs *ipfsnode.Shell, log logger.Logger, bootStrap []string, peerID string) *PeerManager {
	p := &PeerManager{
		peerID:    peerID,
		ipfs:      ipfs,
		log:       log.Named("PeerManager"),
		ps:        make([]bool, maxNumPort),
		startPort: startPort,
		lport:     lport,
		bootStrap: bootStrap,
	}
	for _, bs := range p.bootStrap {
		_, bsID := path.Split(bs)
		err := p.ipfs.SwarmConnect(context.Background(), "/ipfs/"+bsID)
		if err == nil {
			p.log.Info("Bootstrap swarm connected")
		} else {
			p.log.Error("Bootstrap swarm failed to connect")
		}
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
	offset := uint16(port) - pm.startPort
	if int(offset) >= len(pm.ps) {
		return false
	}
	pm.ps[offset] = false
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

func (pm *PeerManager) OpenPeerConn(peerID string, did string, appname string) (*Peer, error) {
	// local peer
	if peerID == pm.peerID {
		var err error
		p := &Peer{
			pm:     pm,
			local:  true,
			log:    pm.log.Named(peerID),
			peerID: peerID,
			did:    did,
		}
		scfg := &srvcfg.Config{
			ServerAddress: "localhost",
			ServerPort:    fmt.Sprintf("%d", pm.lport),
		}
		p.Client, err = ensweb.NewClient(scfg, p.log)
		if err != nil {
			pm.log.Error("failed to create ensweb clent", "err", err)
			return nil, err
		}
		return p, nil
	} else {
		if !pm.SwarmConnect(peerID) {
			pm.log.Error("Failed to connect swarm peer", "peerID", peerID)
			return nil, fmt.Errorf("failed to connect swarm peer")
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
			port:   portNum,
			pm:     pm,
			log:    pm.log.Named(peerID),
			peerID: peerID,
			did:    did,
		}
		proto := "/x/" + appname + "/1.0"
		addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", portNum)
		peer := "/p2p/" + peerID
		resp, err := pm.ipfs.Request("p2p/forward", proto, addr, peer).Send(context.Background())
		if err != nil {
			pm.log.Error("failed make forward request")
			pm.releasePeerPort(portNum)
			return nil, err
		}
		defer resp.Close()
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
}

func (p *Peer) SendJSONRequest(method string, path string, querry map[string]string, req interface{}, resp interface{}, did bool, timeout ...time.Duration) error {
	httpReq, err := p.JSONRequest(method, path, req)
	httpReq.Close = true
	if err != nil {
		return err
	}
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
		p.log.Error("failed to receive reply", "err", err)
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

func (p *Peer) IsLocal() bool {
	return p.local
}

func (p *Peer) Close() error {
	if !p.local {
		defer p.pm.releasePeerPort(p.port)
		addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", p.port)
		req := p.pm.ipfs.Request("p2p/close")
		resp, err := req.Option("listen-address", addr).Send(context.Background())
		if err != nil {
			p.log.Error("failed to close ipfs port", "err", err)
			return err
		}
		defer resp.Close()
		if resp.Error != nil {
			p.log.Error("failed to close ipfs port", "err", resp.Error)
			return resp.Error
		}
	}
	return nil
}

func (p *Peer) GetPeerDID() string {
	return p.did
}
