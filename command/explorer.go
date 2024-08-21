package command

import "fmt"

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
