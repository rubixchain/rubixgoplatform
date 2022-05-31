package core

import (
	"encoding/json"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	OracleTopic string = "oracle"
)

func (c *Core) OracleSubscribe() error {
	return c.ps.SubscribeTopic(OracleTopic, c.oracleCallback)
}

func (c *Core) oracleCallback(msg *ipfsnode.Message) {
	var input model.Input
	var data []byte = msg.Data
	var peerID peer.ID = msg.From
	err := json.Unmarshal(data, &input)
	if err != nil {
		c.log.Error("failed to parse pubsub data", "err", err)
		return
	}
	c.oracle(input, peerID)
}

func (c *Core) PublishOracle(input model.Input) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}

	return c.ps.Publish(OracleTopic, string(b))
}
