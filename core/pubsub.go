package core

import (
	"encoding/json"
	"fmt"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	ExploreTopic string = "explorer"
)

func (c *Core) ExploreSubscribe() error {
	return c.ps.SubscribeTopic(ExploreTopic, c.exploreCallback)
}

func (c *Core) exploreCallback(msg *ipfsnode.Message) {
	var exp model.ExploreModel
	var data []byte = msg.Data
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
