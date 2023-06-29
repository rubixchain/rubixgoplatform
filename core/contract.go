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

func (c *Core) PublishNewEvent(newEvent *model.NewContractEvent) {
	c.publishNewEvent(newEvent)
}

func (c *Core) publishNewEvent(newEvent *model.NewContractEvent) error {
	topic := newEvent.Contract
	if c.pubsub != nil {
		err := c.pubsub.Publish(topic, newEvent)
		c.log.Info("new state published on contract " + topic)
		if err != nil {
			c.log.Error("Failed to publish new event", "err", err)
		}
	}
	return nil
}

func (c *Core) SubsribeContractSetup(topic string) error {
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	return c.pubsub.SubscribeTopic(topic, c.ContractCallBack)
}

func (c *Core) ContractCallBack(peerID string, topic string, data []byte) {
	var newEvent model.NewContractEvent
	err := json.Unmarshal(data, &newEvent)
	c.log.Info("Contract Update")
	if err != nil {
		c.log.Error("Failed to get contract details", "err", err)
	}
	c.log.Info("Contract owner is " + newEvent.Did)
	c.log.Info("Contract hash is " + newEvent.Contract)
	c.log.Info("New block published is " + newEvent.ContractBlockHash)
}
