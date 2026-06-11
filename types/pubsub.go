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

// subscription holds the per-topic state needed to dispatch messages and to
// stop the receiver goroutine on Unsubscribe.
type subscription struct {
	cb   PubSubCallback
	p    *ipfsnode.PubSubSubscription
	done chan struct{} // closed by Unsubscribe to signal intentional teardown
}

type PubSub struct {
	ipfs *ipfsnode.Shell
	log  logger.Logger

	// mu guards sub. Subscriptions are no longer startup-only: the peer_info
	// topic is subscribed/unsubscribed on the transaction path (per DID miss),
	// concurrently with receivePub readers on other topics. A plain map read
	// racing a delete is a fatal "concurrent map read and map write" panic, so
	// every access to sub MUST hold mu. The lock is held ONLY around the map
	// access itself and released before invoking any callback (see receivePub),
	// so a callback that subscribes/unsubscribes cannot self-deadlock.
	mu  sync.RWMutex
	sub map[string]*subscription

	// Counters for trackedTopic only.
	publishCount int64
	receiveCount int64
	statsOnce    sync.Once
}

func NewPubSub(ipfs *ipfsnode.Shell, log logger.Logger) (*PubSub, error) {
	return &PubSub{ipfs: ipfs, log: log, sub: make(map[string]*subscription)}, nil
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
		go func() {
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
		}()
	})
}

func (ps *PubSub) SubscribeTopic(topic string, cb PubSubCallback) error {
	ps.mu.Lock()
	if _, ok := ps.sub[topic]; ok {
		ps.mu.Unlock()
		ps.log.Error(topic, " - already subscribed")
		return fmt.Errorf("topic already subscribed")
	}
	p, err := ps.ipfs.PubSubSubscribe(topic)
	if err != nil {
		ps.mu.Unlock()
		ps.log.Error(topic, " - failed to subscribe", "err", err)
		return err
	}
	s := &subscription{cb: cb, p: p, done: make(chan struct{})}
	ps.sub[topic] = s
	ps.mu.Unlock()

	if topic == trackedTopic {
		ps.startStatsReporter()
	}
	go ps.receivePub(topic, s)
	return nil
}

// Unsubscribe stops receiving messages on the given topic and tears down the
// receiver goroutine. It is idempotent: unsubscribing a topic that is not
// currently subscribed is a no-op. Closing s.done signals receivePub that the
// subscription error it is about to observe from Cancel() is intentional, so it
// exits instead of busy-looping.
func (ps *PubSub) Unsubscribe(topic string) error {
	ps.mu.Lock()
	s, ok := ps.sub[topic]
	if !ok {
		ps.mu.Unlock()
		return nil
	}
	delete(ps.sub, topic)
	ps.mu.Unlock()

	close(s.done)
	if err := s.p.Cancel(); err != nil {
		ps.log.Debug(topic, " - error cancelling subscription", "err", err)
		return err
	}
	return nil
}

func (ps *PubSub) receivePub(topic string, s *subscription) {
	for {
		m, err := s.p.Next()
		if err != nil {
			// Distinguish an intentional Unsubscribe (s.done closed, Cancel()
			// made Next() return an error) from a transient receive error. On
			// intentional teardown, exit; otherwise keep listening.
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		if topic == trackedTopic {
			//count := atomic.AddInt64(&ps.receiveCount, 1)
			//ps.log.Debug("PUBSUB RECV",
			//	"topic", topic,
			//	"from", m.From.String(),
			//	"bytes", len(m.Data),
			//	"receivedTotal", count)
		}
		// Read the callback under the lock, but invoke it AFTER releasing the
		// lock so a callback that subscribes/unsubscribes cannot deadlock.
		ps.mu.RLock()
		cur, ok := ps.sub[topic]
		ps.mu.RUnlock()
		if ok && cur.cb != nil {
			go cur.cb(m.From.String(), topic, m.Data)
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
