package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/adapter"
	srvcfg "github.com/rubixchain/rubixgoplatform/wrapper/config"
)

const (
	ExplorerService string = "explorer_service"
)

type ServiceDetials struct {
	running bool
	db      *adapter.Adapter
}

const (
	NodeStatusTable string = "NodeStatusTable"
	NodeDIDMapTable string = "NodeDIDMapTable"
)

const (
	ExpPeerStatusCmd string = "PeerStatus"
	ExpDIDPeerMapCmd string = "DIDPeerMap"
)

type ExplorerNodeStatus struct {
	PeerID               string    `gorm:"column:PeerID;primaryKey;"`
	CreationTime         time.Time `gorm:"column:CreationTime;not null"`
	LastModificationTime time.Time `gorm:"column:LastModificationTime;not null"`
	Status               string    `gorm:"column:Status;"`
}

type ExplorerNodeDIDMap struct {
	DID                  string    `gorm:"column:DID;primaryKey;"`
	PeerID               string    `gorm:"column:PeerID;"`
	CreationTime         time.Time `gorm:"column:CreationTime;not null"`
	LastModificationTime time.Time `gorm:"column:LastModificationTime;not null"`
}

func (c *Core) ConfigureService(cfg *config.ServiceConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if c.cfg.CfgData.Services == nil {
		c.cfg.CfgData.Services = make(map[string]string)
	}
	c.cfg.CfgData.Services[cfg.ServiceName] = string(b)
	err = c.updateConfig()
	if err != nil {
		return err
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	err = c.initServices()
	if err != nil {
		return err
	}
	return nil
}

func (c *Core) initServices() error {
	if c.cfg.CfgData.Services == nil {
		return nil
	}

	for sn, scfg := range c.cfg.CfgData.Services {
		var cfg config.ServiceConfig
		err := json.Unmarshal([]byte(scfg), &cfg)
		if err != nil {
			return err
		}
		dcfg := &srvcfg.Config{
			DBName:     cfg.DBName,
			DBAddress:  cfg.DBAddress,
			DBPort:     cfg.DBPort,
			DBType:     cfg.DBType,
			DBUserName: cfg.DBUserName,
			DBPassword: cfg.DBPassword,
		}
		db, err := adapter.NewAdapter(dcfg)
		if err != nil {
			return err
		}
		sd := &ServiceDetials{
			db: db,
		}
		c.lock.Lock()
		c.sd[sn] = sd
		c.lock.Unlock()
		err = c.startService(sn)
		if err != nil {
			c.log.Error("Failed to start service", "err", err)
			return err
		}
	}
	return nil
}

func (c *Core) startService(sn string) error {
	c.lock.Lock()
	sd, ok := c.sd[sn]
	c.lock.Unlock()
	if !ok {
		return fmt.Errorf("failed to get service detials")
	}
	switch sn {
	case ExplorerService:
		err := sd.db.InitTable(NodeStatusTable, &ExplorerNodeStatus{}, false)
		if err != nil {
			return err
		}
		err = sd.db.InitTable(NodeDIDMapTable, &ExplorerNodeDIDMap{}, false)
		if err != nil {
			return err
		}
		sd.running = true
		return c.ps.SubscribeTopic(ExplorerService, c.exploreCallback)
	default:
		return fmt.Errorf("unknown service %s", sn)
	}
}
