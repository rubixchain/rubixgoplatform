package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
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
	// peerPeerID := c.l.GetQuerry(req, "self_peerId")
	// peerDID := c.l.GetQuerry(req, "selfDID")
	// peerDIDType := c.l.GetQuerry(req, "selfDID_type")

	// //If the peer's DID type string is not empty, register the peer, if not already registered
	// if peerDIDType != "" {
	// 	peerDIDTypeInt, err1 := strconv.Atoi(peerDIDType)
	// 	if err1 != nil {
	// 		c.log.Debug("could not convert string to integer:", err1)
	// 	}
	// 	err2 := c.w.AddDIDPeerMap(peerDID, peerPeerID, peerDIDTypeInt)
	// 	if err2 != nil {
	// 		c.log.Debug("could not add quorum details to DID peer table:", err2)
	// 	}
	// }
	exist := c.w.IsDIDExist(did)
	ps := model.PeerStatusResponse{
		Version:   c.version,
		DIDExists: exist,
	}
	return c.l.RenderJSON(req, &ps, http.StatusOK)
}

func (c *Core) getPeer(addr string) (*ipfsport.Peer, error) {
	peerID, did, ok := util.ParseAddress(addr)
	if !ok {
		return nil, fmt.Errorf("invalid address: %v", addr)
	}
	// check if addr contains the peer ID
	if peerID == "" {
		peerInfo, err := c.GetPeerDIDInfo(did)
		if err != nil {
			if peerInfo == nil {
				c.log.Error("could not get peerId of peer ", did, "error", err)
				return nil, fmt.Errorf("could not get peerId of peer %v, error : %v", did, err)
			}
			if strings.Contains(err.Error(), "retry") {
				c.AddPeerDetails(*peerInfo)
			}
		}
		if peerInfo.PeerID == "" {
			c.log.Error("failed to get peerId of peer ", did, "error", err)
			return nil, fmt.Errorf("failed to get peerId of receiver : %v, error: %v", did, err)
		}
		peerID = peerInfo.PeerID
	}
	p, err := c.pm.OpenPeerConn(peerID, did, c.getCoreAppName(peerID))
	if err != nil {
		return nil, err
	}
	q := make(map[string]string)
	q["did"] = did

	// //share self information to the peer, if required
	// if selfDID != "" {
	// 	q["self_peerId"] = c.peerID
	// 	q["selfDID"] = selfDID
	// 	selfDetails, err := c.w.GetDID(selfDID)
	// 	if err != nil {
	// 		c.log.Info("could not fetch did type of peer:", selfDID)
	// 	} else {
	// 		q["selfDID_type"] = strconv.Itoa(selfDetails.Type)
	// 	}
	// }
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
	c.log.Debug("Adding peer details to DIDPeerTable", "peerDetail", *peerDetail.DIDType)
	err := c.w.AddDIDPeerMap(peerDetail.DID, peerDetail.PeerID, *peerDetail.DIDType)
	if err != nil {
		c.log.Error("Failed to add PeerDetails to DIDPeerTable", "err", err)
		return err
	}
	return nil
}

func (c *Core) isDIDInArbitaryAddr(peerDID string) (bool, *wallet.DIDPeerMap, error) {
	arbitaryAddr := []string{"12D3KooWHwsKu3GS9rh5X5eS9RTKGFy6NcdX1bV1UHcH8sQ8WqCM.bafybmicttgw2qx4grueyytrgln35vq2hbyhznv6ks4fabeakm47u72c26u",
		"12D3KooWQ2as3FNtvL1MKTeo7XAuBZxSv8QqobxX4AmURxyNe5mX.bafybmicro2m4kove5vsetej63xq4csobtlzchb2c34lp6dnakzkwtq2mmy",
		"12D3KooWJUJz2ipK78LAiwhc1QUVDvSMjZNBHt4vSAeVAq6FsneA.bafybmics43ef7ldgrogzurh7vukormpgscq4um44bss6mfuopsbjorbyaq",
		"12D3KooWC5fHUg2yzAHydgenodN52MYPKhpK4DKRfS8TSm3idSUV.bafybmif5qnkfnkkrffxvoofah3fjzkmieohjbgyte35rrjrn3goufaiykq",
		"12D3KooWDd7c7DAVb38a9vfCFpqxh5nHbDQ4CYjMJuFfBgzpiagK.bafybmie4iynumz2v3obbtkqirxrejjoljjs3l76frvl43wgalqqgprze6q"}

	basicDID := did.BasicDIDMode
	for _, addr := range arbitaryAddr {
		// Split into two parts: [PeerID, DID]
		arbPeerID, arbDID, ok := util.ParseAddress(addr)
		if !ok {
			c.log.Error("failed to parse asdvisory addr ", addr)
			continue //check if the peerDID matches with any other addr in the list
		}
		// Compare the arbitrary DID (second part) with the peerDID
		if arbDID == peerDID {
			peer := wallet.DIDPeerMap{
				DID:     arbDID,
				PeerID:  arbPeerID,
				DIDType: &basicDID,
			}
			err := c.AddPeerDetails(peer)
			if err != nil {
				c.log.Error("failed to save peer details of Advisory node ", addr)
				return true, &peer, fmt.Errorf("failed to save peer details of Advisory node")
			}
			return true, nil, nil
		}
	}
	return false, nil, nil
}
