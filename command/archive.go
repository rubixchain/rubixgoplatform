package command

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) recoverArchive() {
	recoverArchiveRequest := model.RecoverArchiveReq{
		Did:         cmd.did,
		ArchivePath: cmd.runDir,
	}
	response, err := cmd.c.RecoverArchive(&recoverArchiveRequest)
	if err != nil {
		cmd.log.Error("Failed to Recover Archive : Command execution Failed", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to Recover Archive : Command execution Failed", "err", err)
		return
	}
	cmd.log.Info(response.Message)
	// Print response message
	cmd.log.Info("Archive Recovered Successfully")
}

func (cmd *Command) archive() {
	archiveRequest := model.RecoverArchiveReq{
		Did:         cmd.did,
		ArchivePath: cmd.runDir,
	}
	response, err := cmd.c.Archive(&archiveRequest)
	if err != nil {
		cmd.log.Error("Failed to Archive : Command execution Failed", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to Archive : Command execution Failed", "err", err)
		return
	}
	cmd.log.Info(response.Message)
	// Print response message
	cmd.log.Info("Archived Successfully")
}
