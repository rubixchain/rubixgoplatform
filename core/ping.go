package core

import (
	"context"
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// getPingAppName is called to get apllication name for ping
func getPingAppName(prefix string) string {
	return prefix + "Ping"
}

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
	c.l.AddRoute(APIPingPath, "POST", c.PingRecevied)
}

// PingRecevied is the handler for ping request
func (c *Core) PingRecevied(req *ensweb.Request) *ensweb.Result {
	var pingReq PingRequest
	err := c.l.ParseJSON(req, &pingReq)
	if err != nil {
		return c.l.RenderJSONError(req, http.StatusBadRequest, InvalidPasringErr, InvalidPasringErr)
	}
	resp := &PingResponse{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Ping Received",
		},
	}
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

// PingPeer will ping the peer & get the response
func (c *Core) PingPeer(peerdID string) (string, error) {
	cfg := &ipfsport.Config{
		AppName: getPingAppName(peerdID),
		Port:    c.cfg.CfgData.Ports.ReceiverPort + 11,
		PeerID:  peerdID,
	}

	err := c.ipfs.SwarmConnect(context.Background(), "/ipfs/"+peerdID)
	if err != nil {
		c.log.Error("Failed to connect swarm peer", "err", err)
		return "", err
	}
	cl, err := ipfsport.NewClient(cfg, c.log, c.ipfs)
	if err != nil {
		return "", err
	}
	// Close the p2p before exit
	defer cl.Close()
	pingReq := &PingRequest{
		Message: "Ping Request",
	}
	var pingResp PingResponse
	err = cl.SendJSONRequest(APIPingPath, "POST", pingReq, &pingResp)
	if err != nil {
		return "", err
	}
	return pingResp.Message, nil
}
