package core

import (
	"fmt"
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/util"
)

// PingSetup will setup the ping route
func (c *Core) PeerStatusSetup() {
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
}

func (c *Core) peerStatus(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	exist := false
	for _, d := range c.cfg.CfgData.DIDList {
		if did == d {
			exist = true
		}
	}
	ps := model.PeerStatusResponse{
		Version:   c.version,
		DIDExists: exist,
	}
	return c.l.RenderJSON(req, &ps, http.StatusOK)
}

func (c *Core) getPeer(addr string) (*ipfsport.Peer, error) {
	peerID, did, ok := util.ParseAddress(addr)
	if !ok {
		return nil, fmt.Errorf("invalid address")
	}
	// check if addr contains the peer ID
	if peerID == "" {
		// ::TODO:: get the peer id using the pubsub
		return nil, fmt.Errorf("invalid address")
	}
	p, err := c.pm.OpenPeerConn(peerID, did, c.getCoreAppName(peerID))
	if err != nil {
		return nil, err
	}
	q := make(map[string]string)
	q["did"] = did
	var ps model.PeerStatusResponse
	err = p.SendJSONRequest("GET", APIPeerStatus, q, nil, &ps, false)
	if err != nil {
		return nil, err
	}
	if !ps.DIDExists {
		p.Close()
		return nil, fmt.Errorf("did not exist with the peer")
	}
	// TODO:: Valid the peer version before proceesing
	return p, nil
}
