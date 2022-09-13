package command

import (
	"fmt"

	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) addBootStrap() {
	if len(cmd.peers) == 0 {
		cmd.log.Error("Peers required for bootstrap")
		return
	}
	m := model.BootStrapPeers{
		Peers: cmd.peers,
	}
	c, r, err := cmd.basicClient("POST", server.APIAddBootStrap, &m)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return
	}
	defer resp.Body.Close()
	var br server.Response
	err = jsonutil.DecodeJSONFromReader(resp.Body, &br)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Add bootstrap command failed, " + br.Message)
	} else {
		cmd.log.Info("Add bootstrap command finished, " + br.Message)
	}
}

func (cmd *Command) removeBootStrap() {
	if len(cmd.peers) == 0 {
		cmd.log.Error("Peers required for bootstrap")
		return
	}
	m := model.BootStrapPeers{
		Peers: cmd.peers,
	}
	c, r, err := cmd.basicClient("POST", server.APIRemoveBootStrap, &m)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return
	}
	defer resp.Body.Close()
	var br server.Response
	err = jsonutil.DecodeJSONFromReader(resp.Body, &br)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Remove bootstrap command failed, " + br.Message)
	} else {
		cmd.log.Info("Remove bootstrap command finished, " + br.Message)
	}
}

func (cmd *Command) removeAllBootStrap() {

	c, r, err := cmd.basicClient("POST", server.APIRemoveAllBootStrap, nil)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return

	}
	defer resp.Body.Close()
	var br server.Response
	err = jsonutil.DecodeJSONFromReader(resp.Body, &br)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Remove all bootstrap command failed, " + br.Message)
	} else {
		cmd.log.Info("Remove all bootstrap command finished, " + br.Message)
	}
}

func (cmd *Command) getAllBootStrap() {

	c, r, err := cmd.basicClient("GET", server.APIGetAllBootStrap, nil)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return
	}
	defer resp.Body.Close()
	var br server.BootStrapResponse
	err = jsonutil.DecodeJSONFromReader(resp.Body, &br)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Get all bootstrap command failed, " + br.Message)
	} else {
		cmd.log.Info("Get all bootstrap command finished, " + br.Message)
		fmt.Printf("Response : %v\n", br)
		cmd.log.Info("Bootstrap peers", "peers", br.Result.Peers)
	}
}
