package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type PeerMap struct {
	PeerID    string `json:"peer_id"`
	DID       string `json:"did"`
	DIDAlgo   int64  `json:"did_algo"`
	Signature string `json:"signature"`
	Time      string `json:"time"`
}

// PingSetup will setup the ping route
func (c *Core) peerSetup() error {
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	return c.ps.SubscribeTopic(constants.Event_RubixDID, c.peerCallback)
}

func (c *Core) publishPeerMap(pm *PeerMap) error {
	if c.ps != nil {
		err := c.ps.Publish(constants.Event_RubixDID, pm)
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
	if err != nil {
		c.log.Error("failed to parse explorer data", "err", err)
		return
	}
	// If it is a local DID, no need to create separate did folder or to update DB
	if m.PeerID == c.peerID {
		return
	}

	h := util.CalculateHash([]byte(m.PeerID+m.DID+m.Time), constants.HashAlgorithm_SHA3_256)
	dc, err := c.InitialiseDID(m.DID)
	if err != nil {
		return
	}
	signatureBytes, err := util.Base64ToBytes(m.Signature)
	if err != nil {
		c.log.Error("peerCallback: failed to parse signature, err", err)
		return
	}
	st, err := dc.SignVerify([]byte(h), signatureBytes)
	if err != nil {
		if strings.Contains(err.Error(), "NLSS DID detected") || strings.Contains(err.Error(), "incompatible key format") {
			c.log.Error("NLSS DID detected during peer update. NLSS DIDs are DEPRECATED.", "did", m.DID, "error", err)
		} else {
			c.log.Error("failed signature verification for peer update", "did", m.DID, "err", err)
		}
		return
	}
	if !st {
		return
	}

	didInfo := &models.DID{
		DID:    m.DID,
		PeerID: m.PeerID,
		AlgoID: int64(m.DIDAlgo),
		Local:  false,
	}

	if exists, _ := c.w.IsDIDExists(didInfo.DID); !exists {
		if err := c.w.CreateOrUpdateDID(didInfo); err != nil {
			c.log.Error("peerCallback: failed to update DID information, err: %v", err)
			return
		}
	}
}

func (c *Core) peerStatus(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuery(req, "did")
	// peerPeerID := c.l.GetQuery(req, "self_peerId")
	// peerDID := c.l.GetQuery(req, "selfDID")
	// peerDIDType := c.l.GetQuery(req, "selfDID_type")

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
	exist, err := c.w.IsDIDExists(did)
	if err != nil {

	}
	ps := models.PeerStatusResponse{
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
				c.log.Error("getPeer: could not get peerId of peer ", did, "error", err)
				return nil, fmt.Errorf("getPeer: could not get peerId of peer %v, error : %v", did, err)
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
	var ps models.PeerStatusResponse
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
func (c *Core) AddPeerDetails(peerDetail models.DID) error {
	// Defensive: resolve AlgoID if caller did not set it (zero value).
	// The did_algo table uses 1-based GENERATED ALWAYS AS IDENTITY,
	// so AlgoID=0 will violate the algo_id_fk constraint.
	if peerDetail.AlgoID == 0 {
		algoID, err := c.w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
		if err != nil {
			c.log.Error("AddPeerDetails: failed to resolve default algo ID, skipping DID insert", "did", peerDetail.DID, "err", err)
			return fmt.Errorf("AddPeerDetails: failed to resolve algo ID: %w", err)
		}
		peerDetail.AlgoID = algoID
	}
	err := c.w.CreateOrUpdateDID(&peerDetail)
	if err != nil {
		c.log.Error("Failed to add PeerDetails to DIDPeerTable", "err", err)
		return err
	}
	c.log.Info("PeerDetails added to DIDPeerTable", "did", peerDetail.DID)
	return nil
}
