package core

import (
	"time"
)

const (
	staleLockedTokenCheckInterval = time.Hour
	staleLockedTokenMaxAge        = 10 * time.Minute
)

// startStaleLockedTokenUnlocker launches a background goroutine on quorum nodes
// (after SetupQuorum) that periodically resets stale Locked RBT tokens back to
// Free. This recovers quorum liquidity left locked when pledge or consensus
// handlers fail without running their deferred cleanup.
func (c *Core) startStaleLockedTokenUnlocker() {
	c.staleUnlockerOnce.Do(func() {
		go c.staleLockedTokenUnlocker()
	})
}

func (c *Core) staleLockedTokenUnlocker() {
	c.log.Info("Stale locked token unlocker started",
		"checkInterval", staleLockedTokenCheckInterval,
		"maxAge", staleLockedTokenMaxAge,
	)

	ticker := time.NewTicker(staleLockedTokenCheckInterval)
	defer ticker.Stop()

	for {
		c.releaseStaleLockedTokens()
		<-ticker.C
	}
}

func (c *Core) releaseStaleLockedTokens() {
	if len(c.qc) == 0 {
		return
	}

	released, err := c.w.ReleaseStaleLockedRBTTokens(c.Ctx, staleLockedTokenMaxAge)
	if err != nil {
		c.log.Error("releaseStaleLockedTokens: failed to release stale locked RBT tokens", "err", err)
		return
	}
	if released > 0 {
		c.log.Info("releaseStaleLockedTokens: released stale locked RBT tokens", "count", released)
	}
}
