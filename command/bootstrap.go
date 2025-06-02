package command

import (
	"fmt"
	"strings"
)

func (cmd *Command) addBootStrap() {
	if len(cmd.peers) == 0 {
		cmd.log.Error("Peers required for bootstrap. Use flag -peers to provide peers separated by a ','")
		return
	}
	for _, peer := range cmd.peers {
		if !strings.HasPrefix(peer, "/") {
			cmd.log.Error(fmt.Sprintf("Invalid bootstrap peer : %s", peer))
			return
		}
	}
	msg, status := cmd.c.AddBootStrap(cmd.peers)

	if !status {
		cmd.log.Error("Add bootstrap command failed, " + msg)
	} else {
		cmd.log.Info("Add bootstrap command finished, " + msg)
	}
}

func (cmd *Command) removeBootStrap() {
	if len(cmd.peers) == 0 {
		cmd.log.Error("Peers required for bootstrap. Use flag -peers to provide peers separated by a ','")
		return
	}
	for _, peer := range cmd.peers {
		if !strings.HasPrefix(peer, "/") {
			cmd.log.Error(fmt.Sprintf("Invalid bootstrap peer : %s", peer))
			return
		}
	}
	msg, status := cmd.c.RemoveBootStrap(cmd.peers)
	if !status {
		cmd.log.Error("Remove bootstrap command failed, " + msg)
	} else {
		cmd.log.Info("Remove bootstrap command finished, " + msg)
	}
}

func (cmd *Command) removeAllBootStrap() {
	msg, status := cmd.c.RemoveAllBootStrap()
	if !status {
		cmd.log.Error("Remove all bootstrap command failed, " + msg)
	} else {
		cmd.log.Info("Remove all bootstrap command finished, " + msg)
	}
}

func (cmd *Command) getAllBootStrap() {
	peers, msg, status := cmd.c.GetAllBootStrap()
	if !status {
		cmd.log.Error("Get all bootstrap command failed, " + msg)
	} else {
		cmd.log.Info("Get all bootstrap command finished, " + msg)
		cmd.log.Info("Bootstrap peers", "peers", peers)
	}
}
