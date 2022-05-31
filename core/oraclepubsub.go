package core

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	OracleTopic string = "oracle"
)

func (c *Core) OracleSubscribe() error {
	return c.ps.SubscribeTopic(OracleTopic, c.oracleCallback)
}

func (c *Core) oracleCallback(data []byte) {
	var input model.Input
	err := json.Unmarshal(data, &input)
	if err != nil {
		c.log.Error("failed to parse pubsub data", "err", err)
		return
	}
	fmt.Printf("Message : %v\n", input)
}

func (c *Core) PublishOracle(input model.Input) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}

	return c.ps.Publish(OracleTopic, string(b))
}
