package command

import (
	"fmt"
	"regexp"
	"strings"
)

func (cmd *Command) addExplorer() {
	if len(cmd.links) == 0 {
		cmd.log.Error("provide explorer links required to add")
		return
	}
	msg, status := cmd.c.AddExplorer(cmd.links)

	if !status {
		cmd.log.Error("Add Explorer command failed, " + msg)
	} else {
		cmd.log.Info("Add Explorer command finished, " + msg)
	}
}

func (cmd *Command) removeExplorer() {
	if len(cmd.links) == 0 {
		cmd.log.Error("provide explorer links required to remove")
		return
	}
	msg, status := cmd.c.RemoveExplorer(cmd.links)
	if !status {
		cmd.log.Error("Remove Explorer command failed, " + msg)
	} else {
		cmd.log.Info("Remove Explorer command finished, " + msg)
	}
}

func (cmd *Command) getAllExplorer() {
	links, msg, status := cmd.c.GetAllExplorer()
	if !status {
		cmd.log.Error("Get all Explorer command failed, " + msg)
	} else {
		cmd.log.Info("Get all Explorer command finished, " + msg)
		cmd.log.Info("Explorer links", "links", links)
		for i, q := range links {
			fmt.Printf("URL %d: %s\n", i, q)
		}
	}
}

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
