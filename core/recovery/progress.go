package recovery

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Recovery run states stored in recovery_progress.status.
const (
	recoveryStatusInProgress = "in_progress"
	recoveryStatusCompleted  = "completed"
)

// RecoveryProgress is one node-side recovery run record. It drives resume (the
// transaction cursor to restart from) and the operator status read.
type RecoveryProgress struct {
	DID          string    `json:"did"`
	Mode         string    `json:"mode"`
	Phase        string    `json:"phase"`
	LastTokenID  string    `json:"last_token_id,omitempty"`
	LastPosition int64     `json:"last_position,omitempty"`
	LastTxID     string    `json:"last_tx_id,omitempty"`
	Status       string    `json:"status"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SaveRecoveryProgress upserts the in-progress record for did, storing the cursor
// to resume from. It is called at each transaction page so a crashed run can pick
// up where it stopped.
func (s *Store) SaveRecoveryProgress(ctx context.Context, did, mode string, cursor RecoveryCursor) error {
	if did == "" {
		return fmt.Errorf("SaveRecoveryProgress: did is required")
	}
	if ctx == nil {
		ctx = s.w.Ctx
	}
	phase := cursor.Phase
	if phase == "" {
		phase = PhaseTokens
	}
	if _, err := s.w.Pool().Exec(ctx, `
		INSERT INTO recovery_progress (did, mode, phase, last_token_id, last_position, last_tx_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (did) DO UPDATE SET
			mode          = EXCLUDED.mode,
			phase         = EXCLUDED.phase,
			last_token_id = EXCLUDED.last_token_id,
			last_position = EXCLUDED.last_position,
			last_tx_id    = EXCLUDED.last_tx_id,
			status        = EXCLUDED.status,
			updated_at    = NOW()
	`, did, mode, phase, cursor.LastTokenID, cursor.LastPosition, cursor.LastTxID, recoveryStatusInProgress); err != nil {
		return fmt.Errorf("SaveRecoveryProgress: upsert (did %q): %w", did, err)
	}
	return nil
}

// GetRecoveryProgress returns the recovery record for did. The bool is false when
// no run has been recorded yet.
func (s *Store) GetRecoveryProgress(ctx context.Context, did string) (RecoveryProgress, bool, error) {
	var p RecoveryProgress
	if did == "" {
		return p, false, fmt.Errorf("GetRecoveryProgress: did is required")
	}
	if ctx == nil {
		ctx = s.w.Ctx
	}
	err := s.w.Pool().QueryRow(ctx, `
		SELECT did, mode, phase, last_token_id, last_position, last_tx_id, status, updated_at
		FROM recovery_progress WHERE did = $1
	`, did).Scan(&p.DID, &p.Mode, &p.Phase, &p.LastTokenID, &p.LastPosition, &p.LastTxID, &p.Status, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return p, false, nil
	}
	if err != nil {
		return p, false, fmt.Errorf("GetRecoveryProgress: query (did %q): %w", did, err)
	}
	return p, true, nil
}

// CompleteRecoveryProgress marks did's recovery run finished. A missing row is
// not an error, since a full run may complete without a persisted cursor.
func (s *Store) CompleteRecoveryProgress(ctx context.Context, did string) error {
	if did == "" {
		return fmt.Errorf("CompleteRecoveryProgress: did is required")
	}
	if ctx == nil {
		ctx = s.w.Ctx
	}
	if _, err := s.w.Pool().Exec(ctx, `
		INSERT INTO recovery_progress (did, mode, phase, status, created_at, updated_at)
		VALUES ($1, '', $2, $3, NOW(), NOW())
		ON CONFLICT (did) DO UPDATE SET
			status     = EXCLUDED.status,
			updated_at = NOW()
	`, did, PhaseTx, recoveryStatusCompleted); err != nil {
		return fmt.Errorf("CompleteRecoveryProgress: update (did %q): %w", did, err)
	}
	return nil
}
