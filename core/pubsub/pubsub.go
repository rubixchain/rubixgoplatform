package pubsub

import (
	"encoding/json"
	"fmt"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

type PubSubCallback func(peerID string, topic string, data []byte)

type PubSub struct {
	ipfs *ipfsnode.Shell
	log  logger.Logger
	sub  map[string]PubSubCallback
}

func NewPubSub(ipfs *ipfsnode.Shell, log logger.Logger) (*PubSub, error) {
	return &PubSub{ipfs: ipfs, log: log, sub: make(map[string]PubSubCallback)}, nil
}

func (ps *PubSub) SubscribeTopic(topic string, cb PubSubCallback) error {
	f := ps.sub[topic]
	if f != nil {
		ps.log.Error("topic already subscribed")
		return fmt.Errorf("topic already subscribed")
	}
	ps.sub[topic] = cb
	p, err := ps.ipfs.PubSubSubscribe(topic)
	if err != nil {
		ps.log.Error("topic failed to subscribe", "err", err)
		return err
	}
	go ps.receivePub(topic, p)
	return nil
}
func (ps *PubSub) receivePub(topic string, p *ipfsnode.PubSubSubscription) {
	for {
		m, err := p.Next()
		if err != nil {
			//ps.log.Error("failed to read message", "err", err)
			// if strings.Contains(err.Error(), "An existing connection was forcibly closed by the remote host") {
			// 	break
			// }
			continue
		}
		cb := ps.sub[topic]
		if cb != nil {
			go cb(m.From.String(), topic, m.Data)
		}
	}
}

func (ps *PubSub) Publish(topic string, model interface{}) error {
	b, err := json.Marshal(model)
	if err != nil {
		return err
	}
	return ps.ipfs.PubSubPublish(topic, string(b))
}
