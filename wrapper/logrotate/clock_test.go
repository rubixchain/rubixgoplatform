package logrotate

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually driven Clock, so that the rotation tests never have
// to wait for a real rotation period to elapse.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
}

type fakeWaiter struct {
	deadline time.Time
	ch       chan time.Time
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan time.Time, 1)
	deadline := c.now.Add(d)
	if !deadline.After(c.now) {
		ch <- c.now
		return ch
	}

	c.waiters = append(c.waiters, fakeWaiter{deadline: deadline, ch: ch})
	return ch
}

// Advance moves the clock forward and releases every waiter which is due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)

	pending := c.waiters[:0]
	for _, w := range c.waiters {
		if w.deadline.After(c.now) {
			pending = append(pending, w)
			continue
		}
		w.ch <- c.now
	}
	c.waiters = pending
}

// AdvanceTo moves the clock forward to the given instant.
func (c *fakeClock) AdvanceTo(ts time.Time) {
	c.Advance(ts.Sub(c.Now()))
}

// waitForWaiters blocks until the scheduler has armed its timer, so that an
// Advance is never missed.
func (c *fakeClock) waitForWaiters(t *testing.T, count int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		armed := len(c.waiters)
		c.mu.Unlock()
		if armed >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for %d scheduled timer(s)", count)
}
