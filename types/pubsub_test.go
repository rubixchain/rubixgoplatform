package types

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func newTestPubSub(buf *bytes.Buffer) *PubSub {
	log := logger.New(&logger.LoggerOptions{
		Name:   "pubsub-test",
		Level:  logger.Debug,
		Color:  []logger.ColorOption{logger.ColorOff},
		Output: []io.Writer{buf},
	})
	return &PubSub{log: log, sub: make(map[string]*subscription)}
}

// A panicking callback must not take the process down; receivePub dispatches with go, so run it the same way.
func TestInvokeCallbackRecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	ps := newTestPubSub(&buf)

	panicking := func(peerID string, topic string, data []byte) {
		panic("boom from callback")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ps.invokeCallback(panicking, "peer-abc", "topic-xyz", []byte("payload"))
	}()
	wg.Wait()

	out := buf.String()
	for _, want := range []string{"pubsub callback panicked", "topic-xyz", "peer-abc", "boom from callback", "invokeCallback"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestInvokeCallbackPassesArgumentsThrough(t *testing.T) {
	var buf bytes.Buffer
	ps := newTestPubSub(&buf)

	var gotPeer, gotTopic string
	var gotData []byte
	cb := func(peerID string, topic string, data []byte) {
		gotPeer, gotTopic, gotData = peerID, topic, data
	}

	ps.invokeCallback(cb, "peer-1", "topic-1", []byte("hello"))

	if gotPeer != "peer-1" || gotTopic != "topic-1" || string(gotData) != "hello" {
		t.Fatalf("callback received (%q, %q, %q)", gotPeer, gotTopic, gotData)
	}
	if strings.Contains(buf.String(), "panicked") {
		t.Fatalf("unexpected panic log:\n%s", buf.String())
	}
}

func TestIsSubscribed(t *testing.T) {
	var buf bytes.Buffer
	ps := newTestPubSub(&buf)

	if ps.IsSubscribed("sc-topic") {
		t.Fatal("expected unsubscribed topic to report false")
	}
	ps.sub["sc-topic"] = &subscription{}
	if !ps.IsSubscribed("sc-topic") {
		t.Fatal("expected subscribed topic to report true")
	}
	delete(ps.sub, "sc-topic")
	if ps.IsSubscribed("sc-topic") {
		t.Fatal("expected removed topic to report false")
	}
}
