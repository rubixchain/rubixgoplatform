package core

import (
	"encoding/json"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

func (c *Core) PublishExplorer(exp *model.ExploreModel) error {
	if c.ps != nil {
		err := c.ps.Publish(ExplorerService, exp)
		if err != nil {
			c.log.Error("Failed to publish message to explorer", "err", err)
			return err
		}
	}
	return nil
}

func (c *Core) exploreCallback(peerID string, topic string, data []byte) {
	c.lock.Lock()
	sd, ok := c.sd[ExplorerService]
	c.lock.Unlock()
	if !ok || !sd.running {
		return
	}
	var exp model.ExploreModel
	err := json.Unmarshal(data, &exp)
	if err != nil {
		c.log.Error("failed to parse explorer data", "err", err)
		return
	}
	switch exp.Cmd {
	case ExpPeerStatusCmd:
		var node ExplorerNodeStatus
		err := sd.db.FindNew(uuid.Nil, NodeStatusTable, "PeerID=?", &node, peerID)
		if err != nil {
			node.PeerID = peerID
			node.CreationTime = time.Now()
			node.LastModificationTime = time.Now()
			node.Status = exp.Status
			err = sd.db.Create(NodeStatusTable, &node)
			if err != nil {
				c.log.Error("Failed to create peer status", "err", err)
				return
			}
		} else {
			node.LastModificationTime = time.Now()
			node.Status = exp.Status
			err = sd.db.UpdateNew(uuid.Nil, NodeStatusTable, "PeerID=?", &node, peerID)
			if err != nil {
				c.log.Error("Failed to update peer status", "err", err)
				return
			}
		}
	case ExpDIDPeerMapCmd:
		var didMap ExplorerNodeDIDMap
		for _, did := range exp.DIDList {
			err := sd.db.FindNew(uuid.Nil, NodeDIDMapTable, "DID=?", &didMap, did)
			if err != nil {
				didMap.DID = did
				didMap.PeerID = peerID
				didMap.CreationTime = time.Now()
				didMap.LastModificationTime = time.Now()
				err = sd.db.Create(NodeDIDMapTable, &didMap)
				if err != nil {
					c.log.Error("Failed to create did map table", "err", err)
					return
				}
			} else {
				didMap.LastModificationTime = time.Now()
				didMap.PeerID = peerID
				err = sd.db.UpdateNew(uuid.Nil, NodeDIDMapTable, "DID=?", &didMap, did)
				if err != nil {
					c.log.Error("Failed to update did map table", "err", err)
					return
				}
			}
		}

	}
}
