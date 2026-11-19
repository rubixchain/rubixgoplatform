package core

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
)

// recoveryDeadline returns the Unix time, in seconds, at which a session
// whose request has the given epoch moves to state: SyncOpen when the commit
// window closes, SyncClosed when the sync window closes. Both include the
// grace period for clock drift (Q6).
func recoveryDeadline(requestEpoch int64, state int) int64 {
	at := requestEpoch + constants.RecoveryWindow_CommitSeconds + constants.RecoveryWindow_GraceSeconds
	if state == constants.RecoveryState_SyncClosed {
		at += constants.RecoveryWindow_SyncSeconds
	}
	return at
}

// scheduleRecoveryDeadlines starts a timer for each window of s that has not
// closed yet. A deadline already in the past fires at once, so a session
// restored after a restart catches up.
func (c *Core) scheduleRecoveryDeadlines(s *types.RecoverySession) {
	now := time.Now().Unix()
	for _, state := range []int{constants.RecoveryState_SyncOpen, constants.RecoveryState_SyncClosed} {
		if s.State >= state {
			continue
		}
		requestID, to := s.RequestID, state
		delay := time.Duration(recoveryDeadline(s.RequestEpoch, to)-now) * time.Second
		if delay < 0 {
			delay = 0
		}
		time.AfterFunc(delay, func() { c.closeRecoveryWindow(requestID, to) })
	}
}

// closeRecoveryWindow moves a session to SyncOpen or SyncClosed when its
// deadline passes. It changes nothing if the session is already at or past
// that state, or is blocked. Closing the commit window starts the fullnode
// audit.
func (c *Core) closeRecoveryWindow(requestID string, to int) {
	// After a restart both timers can fire at once. Close the commit window
	// first, so the audit is not skipped when the sync deadline wins.
	if to == constants.RecoveryState_SyncClosed {
		c.closeRecoveryWindow(requestID, constants.RecoveryState_SyncOpen)
	}
	advanced, err := c.w.AdvanceRecoverySession(requestID, to)
	if err != nil {
		c.log.Error("Failed to close recovery window", "request_txn_id", requestID, "state", to, "err", err)
		return
	}
	if !advanced {
		return
	}
	if to == constants.RecoveryState_SyncOpen {
		c.log.Info("Recovery commit window closed", "request_txn_id", requestID)
		c.auditRecoveryCommitments(requestID)
	} else {
		c.log.Info("Recovery sync window closed", "request_txn_id", requestID)
	}
}

// restoreRecoverySessions resubscribes to the ephemeral NFT of every open
// session and restarts its deadline timers. It runs at startup, because
// subscriptions and timers do not survive a restart.
func (c *Core) restoreRecoverySessions() {
	sessions, err := c.w.GetOpenRecoverySessions()
	if err != nil {
		c.log.Error("Failed to restore recovery sessions", "err", err)
		return
	}
	for _, s := range sessions {
		c.subscribeRecoveryTopic(s.EphemeralNFTID)
		c.scheduleRecoveryDeadlines(s)
	}
	if len(sessions) > 0 {
		c.log.Info("Recovery sessions restored", "count", len(sessions))
	}
}
