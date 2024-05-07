package core

import (
	"fmt"
	"net/http"
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
