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

var reqID string

func (c *Core) PublishNewEvent(nc *model.NewContractEvent) {
	c.publishNewEvent(nc)
}

func (c *Core) publishNewEvent(newEvent *model.NewContractEvent) error {
	topic := newEvent.Contract
	if c.ps != nil {
		err := c.ps.Publish(topic, newEvent)
		c.log.Info("new state published on contract " + topic)
		if err != nil {
			c.log.Error("Failed to publish new event", "err", err)
		}
	}
	return nil
}

func (c *Core) SubsribeContractSetup(requestID string, topic string) error {
	reqID = requestID
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	return c.ps.SubscribeTopic(topic, c.ContractCallBack)
}

func (c *Core) ContractCallBack(peerID string, topic string, data []byte) {
	var newEvent model.NewContractEvent
	var fetchSC FetchSmartContractRequest
	requestID := reqID
	err := json.Unmarshal(data, &newEvent)
	c.log.Info("Contract Update")
	if err != nil {
		c.log.Error("Failed to get contract details", "err", err)
	}
	fetchSC.SmartContractToken = newEvent.Contract
	fetchSC.SmartContractTokenPath, err = c.CreateSCTempFolder()
	if err != nil {
		c.log.Error("Fetch smart contract failed, failed to create smartcontract folder", "err", err)
		return
	}
	fetchSC.SmartContractTokenPath, err = c.RenameSCFolder(fetchSC.SmartContractTokenPath, fetchSC.SmartContractToken)
	if err != nil {
		c.log.Error("Fetch smart contract failed, failed to create SC folder", "err", err)
		return
	}
	c.FetchSmartContract(requestID, &fetchSC)
}
