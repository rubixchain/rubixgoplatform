package core

import (
	"encoding/json"
	"fmt"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	OracleTopic    string = "oracle"
	ResponsesCount int    = 3
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
	fmt.Printf("%+v\n", input)
	if c.peerID != peerID.String() {
		c.oracle(input, peerID)
	}

}

func (c *Core) PublishOracle(input model.Input) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	c.oracleFlag = true
	c.param = nil
	err = c.ps.Publish(OracleTopic, string(b))

	if err != nil {
		return err
	}

	result := make(chan bool, 1)
	go func() {
		result <- c.CheckParamLen(c.param)
	}()
	select {
	case <-time.After(10 * time.Second):
		fmt.Println("Timed out, 10 seconds up, couldn't fetch ", ResponsesCount, "responses.")
		fmt.Println("Fetched ", len(c.param), "responses.")
		c.oracleFlag = false
	case <-result:
		fmt.Println("Received ", len(c.param), "responses.")
		c.oracleFlag = false
	}
	c.ValidateResponses(input, c.param)
	return err
}

func (c *Core) CheckParamLen(item []interface{}) bool {
	for true {
		if len(c.param) == ResponsesCount {
			break
		}
	}
	return true
}
