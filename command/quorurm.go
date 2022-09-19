package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) AddQuorurm() {

	if cmd.quorumList == "" {
		cmd.log.Error("Quorum list required")
		return
	}
	qlb, err := ioutil.ReadFile(cmd.quorumList)
	if err != nil {
		cmd.log.Error("Invalid file", "err", err)
		return
	}
	var ql model.QuorumList
	err = json.Unmarshal(qlb, &ql)
	if err != nil {
		cmd.log.Error("Invalid quorum list", "err", err)
		return
	}
	c, r, err := cmd.basicClient("POST", server.APIAddQuorum, &ql)
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
	var response server.Response
	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}

	if !response.Status {
		cmd.log.Error("Failed to add quorum list to node", "msg", response.Message)
		return
	}
	cmd.log.Info("Quorum list added successfully")
}

func (cmd *Command) GetAllQuorum() {
	c, r, err := cmd.basicClient("GET", server.APIGetAllQuorum, nil)
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
	var response server.QuorumListResponse
	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}

	if !response.Status {
		cmd.log.Error("Failed to get quorum list from node", "msg", response.Message)
		return
	}
	for _, q := range response.Result.Quorum {
		fmt.Printf("Type : %d, Address : %s\n", q.Type, q.Address)
	}
	cmd.log.Info("Got all quorum list successfully")
}
