package core

import (
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types/models"
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

type GetPeerInfoResponse struct {
	PeerInfo models.DID
	model.BasicResponse
}

// PingSetup will setup the ping route
func (c *Core) PingSetup() {
	c.l.AddRoute(APIPingPath, "GET", c.PingRecevied)
	c.l.AddRoute(APIGetPeerInfoPath, "GET", c.GetPeerInfoResponse)
}

// CheckQuorumStatusSetup will setup the ping route
func (c *Core) CheckQuorumStatusSetup() {
	c.l.AddRoute(APICheckQuorumStatusPath, "GET", c.CheckQuorumStatusResponse)
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
	did := c.l.GetQuery(req, "did")
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
	if peerID == "" {
		fmt.Println("peerID is empty in CheckQuorumStatus")
		peerID = c.qm.GetPeerID(did, c.peerID)
		if peerID == "" {
			qPeerDIDInfo, err := c.GetPeerDIDInfo(did)
			if err != nil {
				return "Quorum Connection Error 1", false, fmt.Errorf("1 unable to find Quorum DID info and peer for %v", did)
			}
			peerID = qPeerDIDInfo.PeerID
		}
	}
	if peerID == "" {
		return "Quorum Connection Error", false, fmt.Errorf("2 unable to find Quorum DID info and peer for %v", did)
	}
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

func (c *Core) GetPeerInfoResponse(req *ensweb.Request) *ensweb.Result { //PingRecevied
	//fetch peer details from DIDPeerTable
	peerDID := c.l.GetQuery(req, "did")

	resp := &GetPeerInfoResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	var pInfo models.DID

	pInfo.PeerID = c.w.GetPeerID(peerDID)
	if pInfo.PeerID == "" {
		_, err := c.w.GetDID(peerDID)
		if err != nil {
			c.log.Error("sender does not have prev pledged quorum in DIDPeerTable", peerDID)
			resp.Message = "Couldn't fetch peer id for did: " + peerDID
			resp.Status = false
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		} else {
			pInfo.PeerID = c.peerID
		}
	}

	resp.PeerInfo = pInfo
	resp.Status = true
	resp.Message = "successfully fetched peer details"
	return c.l.RenderJSON(req, &resp, http.StatusOK)

}

func (c *Core) GetPeerInfo(p *ipfsport.Peer, peerDID string) (GetPeerInfoResponse, error) {
	q := make(map[string]string)
	q["did"] = peerDID

	var response GetPeerInfoResponse
	err := p.SendJSONRequest("GET", APIGetPeerInfoPath, q, nil, &response, false)
	return response, err
}
