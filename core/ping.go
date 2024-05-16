package core

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// PingRequest is the model for ping request
type PingRequest struct {
	Message string `json:"message"`
}

// PingResponse is the model for ping response
type PingResponse struct {
	model.BasicResponse
}

// PingSetup will setup the ping route
func (c *Core) PingSetup() {
	c.l.AddRoute(APIPingPath, "GET", c.PingRecevied)
}

// CheckQuorumStatusSetup will setup the ping route
func (c *Core) CheckQuorumStatusSetup() {
	c.l.AddRoute(APICheckQuorumStatusPath, "GET", c.CheckQuorumStatusResponse)
}

// GetPeerdidTypeSetup will setup the ping route
func (c *Core) GetPeerdidTypeSetup() {
	c.l.AddRoute(APIGetPeerDIDTypePath, "GET", c.GetPeerdidTypeResponse)
}

// PingRecevied is the handler for ping request
func (c *Core) PingRecevied(req *ensweb.Request) *ensweb.Result {
	c.log.Info("Ping Received")
	resp := &PingResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	resp.Message = "Ping Received"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

// PingPeer will ping the peer & get the response
func (c *Core) PingPeer(peerID string) (string, error) {
	p, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		return "", err
	}
	// Close the p2p before exit
	defer p.Close()
	var pingResp PingResponse
	err = p.SendJSONRequest("GET", APIPingPath, nil, nil, &pingResp, false, 2*time.Minute)
	if err != nil {
		return "", err
	}
	return pingResp.Message, nil
}

// CheckQuorumStatusResponse is the handler for CheckQuorumStatus request
func (c *Core) CheckQuorumStatusResponse(req *ensweb.Request) *ensweb.Result { //PingRecevied
	did := c.l.GetQuerry(req, "did")
	c.log.Info("Checking Quorum Status")
	resp := &PingResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	_, ok := c.qc[did]
	if !ok {
		c.log.Error("Quorum is not setup")
		resp.Message = "Quorum is not setup"
		resp.Status = false
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	} else {
		resp.Status = true
		resp.Message = "Quorum is setup"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

}

// CheckQuorumStatus will ping the peer & get the response
func (c *Core) CheckQuorumStatus(peerID string, did string) (string, bool, error) { //
	q := make(map[string]string)
	p, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		return "Quorum Connection Error", false, fmt.Errorf("quorum connection error")
	}
	// Close the p2p before exit
	defer p.Close()
	q["did"] = did
	var checkQuorumStatusResponse PingResponse
	err = p.SendJSONRequest("GET", APICheckQuorumStatusPath, q, nil, &checkQuorumStatusResponse, false, 2*time.Minute)
	if err != nil {
		return "Send Json Request error ", false, err
	}
	return checkQuorumStatusResponse.Message, checkQuorumStatusResponse.Status, nil
}

// CheckQuorumStatusResponse is the handler for CheckQuorumStatus request
func (c *Core) GetPeerdidTypeResponse(req *ensweb.Request) *ensweb.Result { //PingRecevied
	did := c.l.GetQuerry(req, "did")
	quorum_peerid := c.l.GetQuerry(req, "quorum_peerid")
	quorum_did := c.l.GetQuerry(req, "quorum_did")
	quorum_did_type := c.l.GetQuerry(req, "quorum_did_type")

	resp := &model.GetDIDTypeResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}

	quorum_did_type_int, err1 := strconv.Atoi(quorum_did_type)
	if err1 != nil {
		c.log.Debug("could not convert string to integer:", err1)
	}

	c.log.Info("registering the quorum:", quorum_did)
	err2 := c.w.AddDIDPeerMap(quorum_did, quorum_peerid, quorum_did_type_int)
	if err2 != nil {
		c.log.Debug("could not add quorum details to DID peer table:", err2)
	}

	dt, err := c.w.GetDID(did)
	if err != nil {
		c.log.Error("Couldn't fetch did type from DID Table", "error", err)
		resp.Message = "Couldn't fetch did type for did: " + did
		resp.Status = false
		resp.DidType = -1
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	} else {
		resp.DidType = dt.Type
		resp.Status = true
		resp.Message = "successfully fetched did type"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

}

// GetPeerdidType will ping the peer & get the did type
func (c *Core) GetPeerdidType_fromPeer(peerID string, peer_did string, quorumDID string) (int, string, error) {
	dt, err0 := c.w.GetDID(quorumDID)
	if err0 != nil {
		c.log.Info("could not fetch did type of quorum:", quorumDID)
	}
	q := make(map[string]string)
	p, err := c.pm.OpenPeerConn(peerID, peer_did, c.getCoreAppName(peerID))
	if err != nil {
		return -1, "Quorum Connection Error", fmt.Errorf("quorum connection error")
	}

	c.log.Info("Fetching peer did type from peer:", peer_did)

	// Close the p2p before exit
	defer p.Close()
	q["did"] = peer_did
	q["quorum_peerid"] = c.peerID
	q["quorum_did"] = quorumDID
	q["quorum_did_type"] = strconv.Itoa(dt.Type)

	var getPeerdidTypeResponse model.GetDIDTypeResponse
	err = p.SendJSONRequest("GET", APIGetPeerDIDTypePath, q, nil, &getPeerdidTypeResponse, false, 2*time.Minute)
	if err != nil {
		return -1, "Send Json Request error ", err
	}
	return getPeerdidTypeResponse.DidType, getPeerdidTypeResponse.Message, nil
}
