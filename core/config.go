package core

import "github.com/rubixchain/rubixgoplatform/core/config"

func (c *Core) SetupDB(sc *config.StorageConfig) error {
	c.cfg.CfgData.StorageConfig.DBAddress = sc.DBAddress
	c.cfg.CfgData.StorageConfig.DBName = sc.DBName
	c.cfg.CfgData.StorageConfig.DBUserName = sc.DBUserName
	c.cfg.CfgData.StorageConfig.DBPassword = sc.DBPassword
	c.cfg.CfgData.StorageConfig.DBPort = sc.DBPort
	c.cfg.CfgData.StorageConfig.DBType = sc.DBType
	c.cfg.CfgData.StorageConfig.StorageType = sc.StorageType
	return c.updateConfig()
}
