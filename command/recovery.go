package command

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func (cmd *Command) StartRecovery() {
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !isAlphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	br, err := cmd.c.StartRecovery(cmd.did)
	if err != nil {
		cmd.log.Error("Failed to start recovery", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to start recovery, " + msg)
		return
	}
	cmd.log.Info(msg)
}

func (cmd *Command) GetRecoverySession() {
	if cmd.txnID == "" {
		cmd.log.Error("Request transaction ID is required (-txnID)")
		return
	}
	resp, err := cmd.c.GetRecoverySession(cmd.txnID)
	if err != nil {
		cmd.log.Error("Failed to get recovery session", "err", err)
		return
	}
	if !resp.Status {
		cmd.log.Error("Failed to get recovery session", "msg", resp.Message)
		return
	}
	out, err := json.MarshalIndent(resp.Result, "", "  ")
	if err != nil {
		cmd.log.Error("Failed to format recovery session", "err", err)
		return
	}
	fmt.Println(string(out))
}
