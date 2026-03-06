package command

import (
	"regexp"
	"strings"
)

func (cmd *Command) addPeerDetailsFromExplorer() {
	if cmd.did == "" {
		cmd.log.Error("DID cannot be empty")
		return
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !isAlphanumeric {
		cmd.log.Error("Invalid DID. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 {
		cmd.log.Error("Invalid DID")
		return
	}
	msg, status := cmd.c.AddPeerDetailsFromExplorer(cmd.did)

	if !status {
		cmd.log.Error("Add peer details from explorer command failed, " + msg)
	} else {
		cmd.log.Info("Add peer details from explorer command finished, " + msg)
	}
}

func (cmd *Command) addUserAPIKey() {
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !isAlphanumeric {
		cmd.log.Error("Invalid DID. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.apiKey == "" {
		cmd.log.Error("API Key cannot be empty")
		return
	}
	msg, status := cmd.c.AddUserAPIKey(cmd.did, cmd.apiKey)

	if !status {
		cmd.log.Error("API Key could not be added, " + msg)
	} else {
		cmd.log.Info("API Key added successfully")
	}
}
