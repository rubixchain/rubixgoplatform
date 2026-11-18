package wallet

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
)

// RecoverySession is this node's view of one recovery request.
type RecoverySession struct {
	RequestID string
	// EphemeralNFTID is the topic of the request's ephemeral NFT. It is a
	// hash, so events on that topic find their session through it.
	EphemeralNFTID string
	UserDID        string
	Role           string // constants.RecoveryRole_*
	State          int    // constants.RecoveryState_*
	// RequestEpoch is the request transaction's epoch. Deadlines count from it.
	RequestEpoch int64
}

// CreateRecoverySession stores a new session in the REQUESTED state.
// It returns false if a session for the request already exists, so a
// duplicate request event is ignored.
func (w *Wallet) CreateRecoverySession(s *RecoverySession) (bool, error) {
	tag, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO recovery_sessions
		     (request_id, ephemeral_nft_id, user_did, role, state, request_epoch)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (request_id) DO NOTHING`,
		s.RequestID, s.EphemeralNFTID, s.UserDID, s.Role, constants.RecoveryState_Requested, s.RequestEpoch,
	)
	if err != nil {
		return false, fmt.Errorf("failed to create recovery session %s: %w", s.RequestID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// GetRecoverySession returns the session for a request, or nil if none.
func (w *Wallet) GetRecoverySession(requestID string) (*RecoverySession, error) {
	return w.getRecoverySession("request_id", requestID)
}

// GetRecoverySessionByEphemeralNFT returns the session whose ephemeral NFT is
// the given topic, or nil if none.
func (w *Wallet) GetRecoverySessionByEphemeralNFT(ephemeralNFTID string) (*RecoverySession, error) {
	return w.getRecoverySession("ephemeral_nft_id", ephemeralNFTID)
}

// getRecoverySession looks a session up by one of its unique columns.
func (w *Wallet) getRecoverySession(column, value string) (*RecoverySession, error) {
	var s RecoverySession
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT request_id, ephemeral_nft_id, user_did, role, state, request_epoch
		 FROM recovery_sessions WHERE `+column+` = $1`,
		value,
	).Scan(&s.RequestID, &s.EphemeralNFTID, &s.UserDID, &s.Role, &s.State, &s.RequestEpoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get recovery session by %s %s: %w", column, value, err)
	}
	return &s, nil
}

// AdvanceRecoverySession moves a session to a later state. It returns false,
// and changes nothing, if the session is missing or already at or past that
// state. A repeated or late event therefore has no effect. A state may be
// skipped, for example when both deadlines passed while the node was down.
func (w *Wallet) AdvanceRecoverySession(requestID string, to int) (bool, error) {
	tag, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE recovery_sessions SET state = $2, updated_at = NOW()
		 WHERE request_id = $1 AND state < $2`,
		requestID, to,
	)
	if err != nil {
		return false, fmt.Errorf("failed to advance recovery session %s: %w", requestID, err)
	}
	return tag.RowsAffected() == 1, nil
}
