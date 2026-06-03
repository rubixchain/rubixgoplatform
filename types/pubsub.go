package types

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

type PubSubCallback func(peerID string, topic string, data []byte)

// trackedTopic is the only topic for which we keep publish/receive
// counters and emit per-message + periodic stats logs. We intentionally
// scope this to the transaction event so unrelated topics (rubix_did,
// remove_rubix_did, token_chain_details, etc.) don't add noise to the logs.
const trackedTopic = constants.Event_RubixTxns

type PubSub struct {
	ipfs *ipfsnode.Shell
	log  logger.Logger
	sub  map[string]PubSubCallback

	// Counters for trackedTopic only.
	publishCount int64
	receiveCount int64
	statsOnce    sync.Once
}

func NewPubSub(ipfs *ipfsnode.Shell, log logger.Logger) (*PubSub, error) {
	return &PubSub{ipfs: ipfs, log: log, sub: make(map[string]PubSubCallback)}, nil
}

// GetTrackedTopicStats returns a snapshot of (published, received) counts
// for the tracked transaction topic.
func (ps *PubSub) GetTrackedTopicStats() (published int64, received int64) {
	return atomic.LoadInt64(&ps.publishCount), atomic.LoadInt64(&ps.receiveCount)
}

// startStatsReporter logs a 30-second cumulative + delta summary at INFO
// level for the tracked topic. Started lazily on first subscribe/publish.
func (ps *PubSub) startStatsReporter() {
	ps.statsOnce.Do(func() {
		//	go func() {
		ticker := time.NewTicker(120 * time.Second)
		defer ticker.Stop()
		var lastPub, lastRcv int64
		for range ticker.C {
			pub := atomic.LoadInt64(&ps.publishCount)
			rcv := atomic.LoadInt64(&ps.receiveCount)
			deltaPub := pub - lastPub
			deltaRcv := rcv - lastRcv
			if pub != 0 || rcv != 0 {
				ps.log.Info("PUBSUB STATS",
					"topic", trackedTopic,
					"publishedTotal", pub,
					"receivedTotal", rcv,
					"publishedDelta30s", deltaPub,
					"receivedDelta30s", deltaRcv)
			}
			lastPub = pub
			lastRcv = rcv
		}
		//}()
	})
}

func (ps *PubSub) SubscribeTopic(topic string, cb PubSubCallback) error {
	f := ps.sub[topic]
	if f != nil {
		ps.log.Error(topic, " - already subscribed")
		return fmt.Errorf("topic already subscribed")
	}
	ps.sub[topic] = cb
	p, err := ps.ipfs.PubSubSubscribe(topic)
	if err != nil {
		ps.log.Error(topic, " - failed to subscribe", "err", err)
		return err
	}
	if topic == trackedTopic {
		ps.startStatsReporter()
	}
	go ps.receivePub(topic, p)
	return nil
}

func (ps *PubSub) receivePub(topic string, p *ipfsnode.PubSubSubscription) {
	for {
		m, err := p.Next()
		if err != nil {
			continue
		}
		if topic == trackedTopic {
			//count := atomic.AddInt64(&ps.receiveCount, 1)
			//ps.log.Debug("PUBSUB RECV",
			//	"topic", topic,
			//	"from", m.From.String(),
			//	"bytes", len(m.Data),
			//	"receivedTotal", count)
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
	if err := ps.ipfs.PubSubPublish(topic, string(b)); err != nil {
		return err
	}
	return nil
}
