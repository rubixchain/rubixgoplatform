package command

import (
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) SetupService() {
	exp := config.ServiceConfig{
		ServiceName: cmd.srvName,
		DBName:      cmd.dbName,
		DBType:      cmd.dbType,
		DBAddress:   cmd.dbAddress,
		DBPort:      cmd.dbPort,
		DBUserName:  cmd.dbUserName,
		DBPassword:  cmd.dbPassword,
	}
	c, r, err := cmd.basicClient("POST", server.APISetupService, &exp)
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
		cmd.log.Error("Failed to setup service", "message", response.Message)
		return
	}
	cmd.log.Info("Service setup successfully")
}
