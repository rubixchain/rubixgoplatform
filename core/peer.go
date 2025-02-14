package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const (
	PeerService string = "peer_service"
)

type PeerMap struct {
	PeerID    string `json:"peer_id"`
	DID       string `json:"did"`
	DIDType   int    `json:"did_type"`
	Signature []byte `json:"signature"`
	Time      string `json:"time"`
}

// PingSetup will setup the ping route
func (c *Core) peerSetup() error {
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	return c.ps.SubscribeTopic(PeerService, c.peerCallback)
}

func (c *Core) publishPeerMap(pm *PeerMap) error {
	if c.ps != nil {
		err := c.ps.Publish(PeerService, pm)
		if err != nil {
			c.log.Error("Failed to publish peer map message", "err", err)
			return err
		}
	}
	return nil
}

func (c *Core) peerCallback(peerID string, topic string, data []byte) {
	var m PeerMap
	err := json.Unmarshal(data, &m)
	c.log.Debug("Peer DID Update")
	if err != nil {
		c.log.Error("failed to parse explorer data", "err", err)
		return
	}
	h := util.CalculateHashString(m.PeerID+m.DID+m.Time, "SHA3-256")
	dc, err := c.InitialiseDID(m.DID, m.DIDType)
	if err != nil {
		return
	}
	st, err := dc.PvtVerify([]byte(h), m.Signature)
	if err != nil || !st {
		return
	}
	c.w.AddDIDPeerMap(m.DID, m.PeerID, m.DIDType)
}

func (c *Core) peerStatus(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	peerPeerID := c.l.GetQuerry(req, "self_peerId")
	peerDID := c.l.GetQuerry(req, "selfDID")
	peerDIDType := c.l.GetQuerry(req, "selfDID_type")

	//If the peer's DID type string is not empty, register the peer, if not already registered
	if peerDIDType != "" {
		peerDIDTypeInt, err1 := strconv.Atoi(peerDIDType)
		if err1 != nil {
			c.log.Debug("could not convert string to integer:", err1)
		}

		err2 := c.w.AddDIDPeerMap(peerDID, peerPeerID, peerDIDTypeInt)
		if err2 != nil {
			c.log.Debug("could not add quorum details to DID peer table:", err2)
		}

	}
	exist := c.w.IsDIDExist(did)
	ps := model.PeerStatusResponse{
		Version:   c.version,
		DIDExists: exist,
	}
	return c.l.RenderJSON(req, &ps, http.StatusOK)
}

func (c *Core) getPeer(addr string, selfDID string) (*ipfsport.Peer, error) {
	peerID, did, ok := util.ParseAddress(addr)
	if !ok {
		return nil, fmt.Errorf("invalid address: %v", addr)
	}
	// check if addr contains the peer ID
	if peerID == "" {
		peerID = c.w.GetPeerID(did)
		if peerID == "" {
			c.log.Error("Peer ID not found", "did", did)
			return nil, fmt.Errorf("invalid address, Peer ID not found")
		}
	}
	p, err := c.pm.OpenPeerConn(peerID, did, c.getCoreAppName(peerID))
	if err != nil {
		return nil, err
	}
	q := make(map[string]string)
	q["did"] = did

	//share self information to the peer, if required
	if selfDID != "" {
		q["self_peerId"] = c.peerID
		q["selfDID"] = selfDID
		selfDetails, err := c.w.GetDID(selfDID)
		if err != nil {
			c.log.Info("could not fetch did type of peer:", selfDID)
		} else {
			q["selfDID_type"] = strconv.Itoa(selfDetails.Type)
		}
	}
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

/*
This methos returns the peer connection to the PeerId supplied as Input.
*/
func (c *Core) connectPeer(peerID string) (*ipfsport.Peer, error) {
	p, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		return nil, err
	}
	/* q := make(map[string]string)
	q["did"] = ""
	var ps model.PeerStatusResponse
	err = p.SendJSONRequest("GET", APIPeerStatus, q, nil, &ps, false)
	if err != nil {
		return nil, err
	}
	if !ps.DIDExists {
		p.Close()
		return nil, fmt.Errorf("did not exist with the peer")
	} */
	// TODO:: Valid the peer version before proceesing
	return p, nil
}

func (c *Core) AddPeerDetails(peerDetail wallet.DIDPeerMap) error {
	err := c.w.AddDIDPeerMap(peerDetail.DID, peerDetail.PeerID, *peerDetail.DIDType)
	if err != nil {
		c.log.Error("Failed to add PeerDetails to DIDPeerTable", "err", err)
		return err
	}
	return nil
}
