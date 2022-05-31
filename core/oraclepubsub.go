package core

import (
	"encoding/json"
	"fmt"

	ipfsnode "github.com/ipfs/go-ipfs-api"
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
	err := json.Unmarshal(data, &input)
	if err != nil {
		c.log.Error("failed to parse pubsub data", "err", err)
		return
	}
	c.oracle(input)
	fmt.Printf("Message : %v\n", input)
}

func (c *Core) PublishOracle(input model.Input) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}

	return c.ps.Publish(OracleTopic, string(b))
}
