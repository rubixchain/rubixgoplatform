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
	c.publishNewEvent(nc)
}

func (c *Core) publishNewEvent(ns *model.NewContractEvent) error {
	topic := ns.Contract
	if c.ps != nil {
		err := c.ps.Publish(topic, ns)
		c.log.Info("new state published on contract " + topic)
		if err != nil {
			c.log.Error("Failed to publish new event", "err", err)
		}
	}
	return nil
}

func (c *Core) SubsribeContractSetup(topic string) error {
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	return c.ps.SubscribeTopic(topic, c.ContractCallBack)
}

func (c *Core) ContractCallBack(peerID string, topic string, data []byte) {
	var cm model.NewContractEvent
	err := json.Unmarshal(data, &cm)
	c.log.Info("Contract Update")
	if err != nil {
		c.log.Error("Failed to get contract details", "err", err)
	}
	c.log.Info("Contract owner is " + cm.Did)
	c.log.Info("Contract hash is " + cm.Contract)
	c.log.Info("New block published is " + cm.ContractBlockHash)
}
