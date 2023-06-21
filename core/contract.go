package core

import (
	"encoding/json"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	NewStateEvent string = "new_state_event"
)

type NewState struct {
	ConOwnerDID  string `json:"contract_ownwer_did"`
	ConHash      string `json:"contract_hash"`
	ConBlockHash string `json:"contract_block_hash"`
}

func (c *Core) PublishNewEvent(nc *model.NewContractEvent) {
	var ns NewState
	ns.ConHash = nc.Contract
	ns.ConOwnerDID = nc.Did
	ns.ConBlockHash = nc.ContractBlockHash
	c.publishNewEvent(&ns)
}

func (c *Core) publishNewEvent(ns *NewState) error {
	topic := ns.ConHash
	BlockHash := ns.ConBlockHash
	if c.ps != nil {
		err := c.ps.Publish(topic, ns)
		c.log.Info("new state published on ", topic)
		c.log.Info("Block hash is ", BlockHash)
		if err != nil {
			c.log.Error("Failed to publish new event", "err", err)
		}
	}
	return nil
}

func (c *Core) SubsribeContractSetup(topic string) error {
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	return c.ps.SubscribeTopic(topic, c.NewEventCallBack)
}

func (c *Core) NewEventCallBack(peerID string, topic string, data []byte) {
	var cm NewState
	err := json.Unmarshal(data, &cm)
	c.log.Info("Contract Update")
	if err != nil {
		c.log.Error("Failed to get contract details", "err", err)
	}
	c.log.Info("Contract owner is ", cm.ConOwnerDID)
	c.log.Info("Contract hash is ", cm.ConHash)
}
