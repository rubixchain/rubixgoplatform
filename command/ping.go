package command

import (
	"time"

	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) ping() {
	if cmd.peerID == "" {
		cmd.log.Error("Required peer id for ping")
		return
	}
	c, r, err := cmd.basicClient("GET", server.APIPing, nil)
	if err != nil {
		cmd.log.Error("Failed to get new client", "err", err)
		return
	}
	q := r.URL.Query()
	q.Add("peerID", cmd.peerID)
	r.URL.RawQuery = q.Encode()
	resp, err := c.Do(r, time.Minute)
	if err != nil {
		cmd.log.Error("Failed to response from the node", "err", err)
		return
	}
	defer resp.Body.Close()
	var model server.Response
	err = jsonutil.DecodeJSONFromReader(resp.Body, &model)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !model.Status {
		cmd.log.Error("Ping failed", "message", model.Message)
	} else {
		cmd.log.Info("Ping response received successfully", "message", model.Message)
	}
}
