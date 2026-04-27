package types

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

type PubSubCallback func(peerID string, topic string, data []byte)

// topicCounters holds per-topic publish / receive counters.
// Both publishCount and receiveCount are accessed via sync/atomic.
type topicCounters struct {
	publishCount int64
	receiveCount int64
}

type PubSub struct {
	ipfs       *ipfsnode.Shell
	log        logger.Logger
	sub        map[string]PubSubCallback
	counters   sync.Map // map[string]*topicCounters
	statsOnce  sync.Once
}

func NewPubSub(ipfs *ipfsnode.Shell, log logger.Logger) (*PubSub, error) {
	ps := &PubSub{ipfs: ipfs, log: log, sub: make(map[string]PubSubCallback)}
	ps.startStatsReporter()
	return ps, nil
}

// getCounters returns the per-topic counter struct, creating it on first use.
func (ps *PubSub) getCounters(topic string) *topicCounters {
	if v, ok := ps.counters.Load(topic); ok {
		return v.(*topicCounters)
	}
	v, _ := ps.counters.LoadOrStore(topic, &topicCounters{})
	return v.(*topicCounters)
}

// GetTopicStats returns a snapshot of (publish, receive) counts for a topic.
func (ps *PubSub) GetTopicStats(topic string) (published int64, received int64) {
	c := ps.getCounters(topic)
	return atomic.LoadInt64(&c.publishCount), atomic.LoadInt64(&c.receiveCount)
}

// startStatsReporter logs a summary of publish/receive counts every 30s,
// for every topic with non-zero traffic. Useful to verify that publishers
// and subscribers are seeing the same number of messages.
func (ps *PubSub) startStatsReporter() {
	ps.statsOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			var lastPub, lastRcv sync.Map // map[string]int64
			for range ticker.C {
				ps.counters.Range(func(key, value interface{}) bool {
					topic := key.(string)
					c := value.(*topicCounters)
					pub := atomic.LoadInt64(&c.publishCount)
					rcv := atomic.LoadInt64(&c.receiveCount)
					var prevPub, prevRcv int64
					if v, ok := lastPub.Load(topic); ok {
						prevPub = v.(int64)
					}
					if v, ok := lastRcv.Load(topic); ok {
						prevRcv = v.(int64)
					}
					deltaPub := pub - prevPub
					deltaRcv := rcv - prevRcv
					if deltaPub != 0 || deltaRcv != 0 || pub != 0 || rcv != 0 {
						ps.log.Info("PUBSUB STATS",
							"topic", topic,
							"publishedTotal", pub,
							"receivedTotal", rcv,
							"publishedDelta30s", deltaPub,
							"receivedDelta30s", deltaRcv)
					}
					lastPub.Store(topic, pub)
					lastRcv.Store(topic, rcv)
					return true
				})
			}
		}()
	})
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
	// Ensure counter struct exists so the stats reporter sees this topic
	// even before any traffic.
	ps.getCounters(topic)
	go ps.receivePub(topic, p)
	return nil
}

func (ps *PubSub) receivePub(topic string, p *ipfsnode.PubSubSubscription) {
	counters := ps.getCounters(topic)
	for {
		m, err := p.Next()
		if err != nil {
			continue
		}
		count := atomic.AddInt64(&counters.receiveCount, 1)
		ps.log.Debug("PUBSUB RECV",
			"topic", topic,
			"from", m.From.String(),
			"bytes", len(m.Data),
			"receivedTotal", count)
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
	count := atomic.AddInt64(&ps.getCounters(topic).publishCount, 1)
	ps.log.Debug("PUBSUB SEND",
		"topic", topic,
		"bytes", len(b),
		"publishedTotal", count)
	return nil
}
