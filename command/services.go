package command

import (
	"github.com/rubixchain/rubixgoplatform/core/config"
)

func (cmd *Command) SetupService() {
	scfg := config.ServiceConfig{
		ServiceName: cmd.srvName,
		DBName:      cmd.dbName,
		DBType:      cmd.dbType,
		DBAddress:   cmd.dbAddress,
		DBPort:      cmd.dbPort,
		DBUserName:  cmd.dbUserName,
		DBPassword:  cmd.dbPassword,
	}
	msg, status := cmd.c.SetupService(&scfg)
	if !status {
		cmd.log.Error("Failed to setup service", "message", msg)
		return
	}
	cmd.log.Info("Service setup successfully")
}
