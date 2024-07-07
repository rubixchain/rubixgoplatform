package command

import (
	"fmt"
	"strings"
)

func (cmd *Command) AddQuorurm() {
	msg, status := cmd.c.AddQuorum(cmd.quorumList)
	if !status {
		cmd.log.Error("Failed to add quorum list to node", "msg", msg)
		return
	}
	cmd.log.Info("Quorum list added successfully")
}

func (cmd *Command) GetAllQuorum() {
	response, err := cmd.c.GettAllQuorum()
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to get quorum list from node", "msg", response.Message)
		return
	}
	for _, q := range response.Result {
		fmt.Printf("Address : %s\n", q)
	}
	cmd.log.Info("Got all quorum list successfully")
}

func (cmd *Command) RemoveAllQuorum() {
	msg, status := cmd.c.RemoveAllQuorum()
	if !status {
		cmd.log.Error("Failed to remove quorum list", "msg", msg)
		return
	}
	cmd.log.Info(msg)
}

func (cmd *Command) SetupQuorum() {
	if cmd.forcePWD {
		pwd, err := getpassword("Enter quorum key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		cmd.quorumPWD = pwd
	}

	if cmd.did == "" {
		cmd.log.Info("Quorum DID cannot be empty")
		fmt.Print("Enter Quorum DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get Quorum DID")
			return
		}
	}
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) < 59 {
		cmd.log.Error("Invalid DID")
		return
	}
	msg, status := cmd.c.SetupQuorum(cmd.did, cmd.quorumPWD, cmd.privPWD)

	if !status {
		cmd.log.Error("Failed to setup quorum", "msg", msg)
		return
	}
	cmd.log.Info("Quorum setup successfully")
}
