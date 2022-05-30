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
	var exp model.ExploreModel
	err := json.Unmarshal(data, &exp)
	if err != nil {
		c.log.Error("failed to parse pubsub data", "err", err)
		return
	}
	fmt.Printf("Message : %v\n", exp)
}

func (c *Core) PublishOracle(exp model.ExploreModel) error {
	b, err := json.Marshal(exp)
	if err != nil {
		return err
	}

	return c.ps.Publish(OracleTopic, string(b))
}
