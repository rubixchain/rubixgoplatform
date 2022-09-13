package command

import (
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) EnableExplorer() {
	exp := config.ExplorerConfig{
		DBName:     cmd.dbName,
		DBType:     cmd.dbType,
		DBAddress:  cmd.dbAddress,
		DBPort:     cmd.dbPort,
		DBUserName: cmd.dbUserName,
		DBPassword: cmd.dbPassword,
	}
	c, r, err := cmd.basicClient("POST", server.APIEnableExplorer, &exp)
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
		cmd.log.Error("Failed to enable explorer service", "message", response.Message)
		return
	}
	cmd.log.Info("Explorer service enabled successfully")
}
