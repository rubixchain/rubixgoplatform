package core

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	ExploreTopic string = "explorer"
)

func (c *Core) ExploreSubscribe() error {
	return c.ps.SubscribeTopic(ExploreTopic, c.exploreCallback)
}

func (c *Core) exploreCallback(data []byte) {
	var exp model.ExploreModel
	err := json.Unmarshal(data, &exp)
	if err != nil {
		c.log.Error("failed to parse pubsub data", "err", err)
		return
	}
	fmt.Printf("Message : %v\n", exp)
}

func (c *Core) PublishExplore(exp model.ExploreModel) error {
	b, err := json.Marshal(exp)
	if err != nil {
		return err
	}
	return c.ps.Publish(ExploreTopic, string(b))
}
