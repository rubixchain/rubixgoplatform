package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// AddProviderDetailsBatch inserts or updates provider details for tokens in a
// single round-trip using pgx.Batch. Each record is an upsert (INSERT ON CONFLICT
// DO UPDATE) keyed on the token PK.
//
// Partial success is tolerated: the method returns an error only if ALL inserts fail.
// This matches the caller expectation in quorum_validation.go where UNIQUE constraint
// collisions are expected when multiple quorums process the same tokens.
func (w *Wallet) AddProviderDetailsBatch(providerMaps []model.TokenProviderMap, tx ...pgx.Tx) error {
	if len(providerMaps) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, pm := range providerMaps {
		batch.Queue(`
			INSERT INTO token_provider_map
				(token, did, func_id, role, transaction_id, initiator, owner, token_value, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
			ON CONFLICT (token) DO UPDATE SET
				did = EXCLUDED.did,
				func_id = EXCLUDED.func_id,
				role = EXCLUDED.role,
				transaction_id = EXCLUDED.transaction_id,
				initiator = EXCLUDED.initiator,
				owner = EXCLUDED.owner,
				token_value = EXCLUDED.token_value,
				updated_at = NOW()`,
			pm.TokenHash, pm.DID, pm.FuncID, pm.Role,
			pm.TransactionID, pm.Initiator, pm.Owner, pm.TokenValue,
		)
	}

	br := w.q(tx...).SendBatch(w.Ctx, batch)
	defer br.Close()

	var firstErr error
	successCount := 0
	for i := 0; i < len(providerMaps); i++ {
		_, err := br.Exec()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successCount++
		}
	}

	if successCount == 0 && firstErr != nil {
		return fmt.Errorf("all provider detail operations failed: %w", firstErr)
	}

	return nil
}

// AddProviderDetails inserts or updates a single provider detail record.
// Pass an optional pgx.Tx to run within a transaction.
func (w *Wallet) AddProviderDetails(tpm model.TokenProviderMap, tx ...pgx.Tx) error {
	_, err := w.q(tx...).Exec(w.Ctx, `
		INSERT INTO token_provider_map
			(token, did, func_id, role, transaction_id, initiator, owner, token_value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (token) DO UPDATE SET
			did = EXCLUDED.did,
			func_id = EXCLUDED.func_id,
			role = EXCLUDED.role,
			transaction_id = EXCLUDED.transaction_id,
			initiator = EXCLUDED.initiator,
			owner = EXCLUDED.owner,
			token_value = EXCLUDED.token_value,
			updated_at = NOW()`,
		tpm.TokenHash, tpm.DID, tpm.FuncID, tpm.Role,
		tpm.TransactionID, tpm.Initiator, tpm.Owner, tpm.TokenValue,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert provider details for token %s: %w", tpm.TokenHash, err)
	}
	return nil
}

// RemoveProviderDetails deletes the provider detail record for the given token and DID.
// Pass an optional pgx.Tx to run within a transaction.
func (w *Wallet) RemoveProviderDetails(token string, did string, tx ...pgx.Tx) error {
	_, err := w.q(tx...).Exec(w.Ctx, `
		DELETE FROM token_provider_map WHERE did = $1 AND token = $2`,
		did, token,
	)
	if err != nil {
		return fmt.Errorf("failed to remove provider details for token %s: %w", token, err)
	}
	return nil
}

// GetProviderDetails retrieves the provider detail record for the given token hash.
// Pass an optional pgx.Tx to run within a transaction.
func (w *Wallet) GetProviderDetails(tokenHash string, tx ...pgx.Tx) (*model.TokenProviderMap, error) {
	var tpm model.TokenProviderMap
	err := w.q(tx...).QueryRow(w.Ctx, `
		SELECT token, did, func_id, role, transaction_id, initiator, owner, token_value
		FROM token_provider_map WHERE token = $1`,
		tokenHash,
	).Scan(&tpm.TokenHash, &tpm.DID, &tpm.FuncID, &tpm.Role,
		&tpm.TransactionID, &tpm.Initiator, &tpm.Owner, &tpm.TokenValue)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider details for token %s: %w", tokenHash, err)
	}
	return &tpm, nil
}
