package core

import (
	"encoding/json"
	"time"

	"github.com/EnsurityTechnologies/adapter"
	srvcfg "github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/uuid"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	NodeStatusTable string = "NodeStatusTable"
	NodeDIDMapTable string = "NodeDIDMapTable"
)

const (
	ExpPeerStatusCmd string = "PeerStatus"
	ExpDIDPeerMapCmd string = "DIDPeerMap"
)

type ExplorerNodeStatus struct {
	PeerID               string    `gorm:"column:PeerID;primary_key;"`
	CreationTime         time.Time `gorm:"column:CreationTime;not null"`
	LastModificationTime time.Time `gorm:"column:LastModificationTime;not null"`
	Status               string    `gorm:"column:Status;"`
}

type ExplorerNodeDIDMap struct {
	DID                  string    `gorm:"column:DID;primary_key;"`
	PeerID               string    `gorm:"column:PeerID;"`
	CreationTime         time.Time `gorm:"column:CreationTime;not null"`
	LastModificationTime time.Time `gorm:"column:LastModificationTime;not null"`
}

func (c *Core) initExplorer(cfg string) error {
	var ecfg config.ExplorerConfig
	err := json.Unmarshal([]byte(cfg), &ecfg)
	if err != nil {
		return err
	}
	dcfg := &srvcfg.Config{
		DBName:     ecfg.DBName,
		DBAddress:  ecfg.DBAddress,
		DBPort:     ecfg.DBPort,
		DBType:     ecfg.DBType,
		DBUserName: ecfg.DBUserName,
		DBPassword: ecfg.DBPassword,
	}
	c.exploreDB, err = adapter.NewAdapter(dcfg)
	if err != nil {
		return err
	}
	err = c.exploreDB.InitTable(NodeStatusTable, &ExplorerNodeStatus{})
	if err != nil {
		return err
	}
	err = c.exploreDB.InitTable(NodeDIDMapTable, &ExplorerNodeDIDMap{})
	if err != nil {
		return err
	}
	if c.explorerStatus {
		return nil
	}
	c.explorerStatus = true
	return c.ps.SubscribeTopic(ExploreTopic, c.exploreCallback)
}

func (c *Core) PublishExplorer(exp *model.ExploreModel) error {
	if c.ps != nil {
		err := c.ps.Publish(ExploreTopic, exp)
		if err != nil {
			c.log.Error("Failed to publish message to explorer", "err", err)
			return err
		}
	}
	return nil
}

func (c *Core) ConfigureExplorer(cfg *config.ExplorerConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if c.cfg.CfgData.Services == nil {
		c.cfg.CfgData.Services = make(map[string]string)
	}
	c.cfg.CfgData.Services[ExploreTopic] = string(b)
	err = c.updateConfig()
	if err != nil {
		return err
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	err = c.initExplorer(c.cfg.CfgData.Services[ExploreTopic])
	if err != nil {
		return err
	}
	return nil
}

func (c *Core) exploreCallback(peerID string, topic string, data []byte) {
	var exp model.ExploreModel
	err := json.Unmarshal(data, &exp)
	if err != nil {
		c.log.Error("failed to parse explorer data", "err", err)
		return
	}
	switch exp.Cmd {
	case ExpPeerStatusCmd:
		var node ExplorerNodeStatus
		err := c.exploreDB.FindNew(uuid.Nil, NodeStatusTable, "PeerID=?", &node, peerID)
		if err != nil {
			node.PeerID = peerID
			node.CreationTime = time.Now()
			node.LastModificationTime = time.Now()
			node.Status = exp.Status
			err = c.exploreDB.Create(NodeStatusTable, &node)
			if err != nil {
				c.log.Error("Failed to create peer status", "err", err)
				return
			}
		} else {
			node.LastModificationTime = time.Now()
			node.Status = exp.Status
			err = c.exploreDB.UpdateNew(uuid.Nil, NodeStatusTable, "PeerID=?", &node, peerID)
			if err != nil {
				c.log.Error("Failed to update peer status", "err", err)
				return
			}
		}
	case ExpDIDPeerMapCmd:
		var didMap ExplorerNodeDIDMap
		for _, did := range exp.DIDList {
			err := c.exploreDB.FindNew(uuid.Nil, NodeDIDMapTable, "DID=?", &didMap, did)
			if err != nil {
				didMap.DID = did
				didMap.PeerID = peerID
				didMap.CreationTime = time.Now()
				didMap.LastModificationTime = time.Now()
				err = c.exploreDB.Create(NodeDIDMapTable, &didMap)
				if err != nil {
					c.log.Error("Failed to create did map table", "err", err)
					return
				}
			} else {
				didMap.LastModificationTime = time.Now()
				didMap.PeerID = peerID
				err = c.exploreDB.UpdateNew(uuid.Nil, NodeDIDMapTable, "DID=?", &didMap, did)
				if err != nil {
					c.log.Error("Failed to update did map table", "err", err)
					return
				}
			}
		}

	}
}
