package core

import (
	"fmt"
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
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

func (c *Core) PingPeerWithBalance(peerID string, did string) (string, error) {
	p, err := c.pm.OpenPeerConn(peerID, did, c.getCoreAppName(peerID))
	if err != nil {
		return "", err
	}
	q := make(map[string]string)
	q["peerID"] = peerID
	q["did"] = did

	var ps model.PeerTokenCountResponse
	err = p.SendJSONRequest("GET", APIGetTokenCount, q, nil, &ps, false)
	if err != nil {
		return "", err
	}
	b := fmt.Sprintf("%v", ps.DIDBalance)
	msg := "Balance of peer ID : " + peerID + " and DID : " + did + " is = " + b
	c.log.Info(msg)
	// Close the p2p before exit
	defer p.Close() /////// Should we close it in the error statement????
	return msg, nil
}
