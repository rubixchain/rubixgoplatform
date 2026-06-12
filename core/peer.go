package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type PeerMap struct {
	PeerID    string `json:"peer_id"`
	DID       string `json:"did"`
	DIDAlgo   int64  `json:"did_algo"`
	Signature string `json:"signature"`
	Time      string `json:"time"`
}

// peerInfoMsgType distinguishes the two halves of the peer_info exchange that
// share a single pubsub topic.
const (
	peerInfoRequest  = "request"
	peerInfoResponse = "response"
)

// peerInfoResolveTimeout bounds how long a requester waits for a fullnode to
// answer a peer_info lookup before failing (and falling back to today's
// "peer ID not found" behaviour).
const peerInfoResolveTimeout = 10 * time.Second

// PeerInfoMsg is the request/response payload exchanged on Event_PeerInfo.
// It is an unauthenticated lookup (no signature): a requester asks "which
// peerID owns this DID?" and any fullnode that knows answers. This sidesteps
// the signing constraint that blocks re-announcing PeerMaps on nodes without
// local private keys.
type PeerInfoMsg struct {
	Type      string `json:"type"`       // peerInfoRequest | peerInfoResponse
	RequestID string `json:"request_id"` // correlation id, set by the requester
	DID       string `json:"did"`        // DID being resolved
	PeerID    string `json:"peer_id"`    // filled only in responses
	DIDAlgo   int64  `json:"did_algo"`   // filled only in responses
	Requester string `json:"requester"`  // requester's peerID
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
		c.log.Info("PeerDetails added to DIDPeerTable via rubix_did announcement", "did", didInfo.DID, "peerID", didInfo.PeerID)
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
	c.log.Info("PeerDetails added to DIDPeerTable", "did", peerDetail.DID, "peerID", peerDetail.PeerID)
	return nil
}

// peerInfoResponderSetup subscribes a fullnode permanently to the peer_info
// topic so it can answer DID->peerID lookups from its authoritative dids
// table. Only fullnodes respond; this is called from SetupCore when
// c.fullNode is true.
func (c *Core) peerInfoResponderSetup() error {
	return c.ps.SubscribeTopic(constants.Event_PeerInfo, c.peerInfoResponderCallback)
}

// peerInfoResponderCallback handles incoming peer_info requests. It runs only
// on fullnodes (the permanent responder subscription). If this node knows the
// requested DID, it publishes a response; otherwise it stays silent so the
// requester simply times out.
func (c *Core) peerInfoResponderCallback(fromPeerID string, topic string, data []byte) {
	var m PeerInfoMsg
	if err := json.Unmarshal(data, &m); err != nil {
		c.log.Error("peerInfoResponderCallback: failed to parse message", "err", err)
		return
	}
	// Only act on requests, and never answer our own echoed request.
	if m.Type != peerInfoRequest {
		return
	}
	if m.Requester == c.peerID {
		return
	}

	didInfo, err := c.w.GetDID(m.DID)
	if err != nil || didInfo.PeerID == "" {
		// We don't know this DID -- stay silent.
		return
	}

	resp := &PeerInfoMsg{
		Type:      peerInfoResponse,
		RequestID: m.RequestID,
		DID:       m.DID,
		PeerID:    didInfo.PeerID,
		DIDAlgo:   didInfo.AlgoID,
	}
	if err := c.ps.Publish(constants.Event_PeerInfo, resp); err != nil {
		c.log.Error("peerInfoResponderCallback: failed to publish response", "did", m.DID, "err", err)
	}
}

// resolvePeerInfoViaPubsub asks the network (fullnodes) for the peerID of a DID
// that is missing from the local dids table. It subscribes to the peer_info
// topic, publishes a request, waits up to peerInfoResolveTimeout for the first
// valid response, persists it, and unsubscribes. Returns ("", false) on timeout
// or any error.
//
// Only non-fullnodes use this path: a fullnode has an authoritative dids table
// and never misses, so it never reaches here. The c.fullNode guard enforces
// that invariant -- a fullnode is already permanently subscribed to peer_info
// (as a responder), so a transient SubscribeTopic here would collide.
func (c *Core) resolvePeerInfoViaPubsub(did string) (string, bool) {
	if c.fullNode {
		return "", false
	}
	if c.ps == nil {
		return "", false
	}

	reqID := uuid.New().String()
	// Buffered so a late/duplicate responder never blocks on send after we've
	// already taken the first answer and moved on.
	resultCh := make(chan PeerInfoMsg, 4)

	cb := func(fromPeerID string, topic string, data []byte) {
		var m PeerInfoMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return
		}
		if m.Type != peerInfoResponse {
			return
		}
		if m.RequestID != reqID || m.DID != did {
			return
		}
		if m.PeerID == "" {
			return
		}
		select {
		case resultCh <- m:
		default:
		}
	}

	if err := c.ps.SubscribeTopic(constants.Event_PeerInfo, cb); err != nil {
		c.log.Error("resolvePeerInfoViaPubsub: failed to subscribe", "did", did, "err", err)
		return "", false
	}
	defer c.ps.Unsubscribe(constants.Event_PeerInfo)

	req := &PeerInfoMsg{
		Type:      peerInfoRequest,
		RequestID: reqID,
		DID:       did,
		Requester: c.peerID,
	}
	if err := c.ps.Publish(constants.Event_PeerInfo, req); err != nil {
		c.log.Error("resolvePeerInfoViaPubsub: failed to publish request", "did", did, "err", err)
		return "", false
	}

	select {
	case m := <-resultCh:
		if err := c.AddPeerDetails(models.DID{
			DID:    m.DID,
			PeerID: m.PeerID,
			AlgoID: m.DIDAlgo,
			Local:  false,
		}); err != nil {
			c.log.Error("resolvePeerInfoViaPubsub: failed to persist resolved peer", "did", did, "err", err)
			return "", false
		}
		c.log.Info("resolvePeerInfoViaPubsub: resolved DID via pubsub", "did", did, "peerID", m.PeerID)
		return m.PeerID, true
	case <-time.After(peerInfoResolveTimeout):
		c.log.Debug("resolvePeerInfoViaPubsub: timed out resolving DID", "did", did)
		return "", false
	}
}
