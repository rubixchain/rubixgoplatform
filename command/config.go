package command

import "github.com/rubixchain/rubixgoplatform/core/config"

func (cmd *Command) setupDB() {
	sc := &config.StorageConfig{
		StorageType: cmd.storageType,
		DBName:      cmd.dbName,
		DBAddress:   cmd.dbAddress,
		DBUserName:  cmd.dbUserName,
		DBPassword:  cmd.dbPassword,
		DBPort:      cmd.dbPort,
		DBType:      cmd.dbType,
	}
	msg, ok := cmd.c.SetupDB(sc)
	if !ok {
		cmd.log.Error("Failed to setup DB", "msg", msg)
		return
	}
	cmd.log.Info("DB setup done successfully")
}
