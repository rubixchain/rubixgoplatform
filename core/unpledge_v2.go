package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// UnpledgeV2 atomically updates tokenchain and token records to reflect the
// unpledge of all tokens associated with mainTxID. Unpledge is a pure DB
// bookkeeping operation — no TransactionInfo, no txID, no signature is created.
//
// Steps performed atomically inside unpledgeTx:
//  1. UPDATE tokenchain SET role=9 WHERE transaction_id=mainTxID AND role=8
//  2. UPDATE tokens SET token_status=FREE, latest_role=9
//
// tokenchain_index is NOT rebuilt — an UPDATE does not change row positions or
// add new rows, so the index remains valid.
//
// Post-commit (separate cleanupTx):
//   - Increment token_denom for quorumDID
//   - DELETE unpledge_sequence_info WHERE tx_id=mainTxID
//
// Idempotency: if no unpledge_sequence_info row exists for mainTxID the
// function returns nil immediately (already unpledged or never pledged via V2).
// Tokens already in FREE status trigger early-return with sequence row cleanup.
//
// Parameters:
//   - mainTxID:   lookup key in unpledge_sequence_info (the main transfer txID)
//   - quorumDID: DID that owns the pledged tokens
//   - proofTx:  transaction info that represents as the proof for which unpledge is being performed. 
func (c *Core) UnpledgeV2(
	ctx context.Context,
	mainTxID string,
	quorumDID string,
	proofTx *models.Transactions,
) error {
	if mainTxID == "" {
		return fmt.Errorf("UnpledgeV2: mainTxID is required")
	}
	if quorumDID == "" {
		return fmt.Errorf("UnpledgeV2: quorumDID is required")
	}

	// --- Idempotency guard: look up unpledge_sequence_info -------------------
	var pledgeTokens []string
	var epoch int
	err := c.w.Pool().QueryRow(ctx,
		`SELECT pledge_tokens, epoch FROM unpledge_sequence_info WHERE tx_id = $1 AND quorum_did = $2`,
		mainTxID, quorumDID,
	).Scan(&pledgeTokens, &epoch)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Either no row exists (idempotent success — already unpledged or
			// never pledged via V2), OR a row exists but is owned by a different
			// quorum DID (mismatch — audit and hard skip). Distinguish the two
			// cases with a second query so the mismatch is not swallowed silently.
			var owner string
			qerr := c.w.Pool().QueryRow(ctx,
				`SELECT quorum_did FROM unpledge_sequence_info WHERE tx_id = $1`,
				mainTxID,
			).Scan(&owner)
			if qerr == pgx.ErrNoRows {
				c.log.Info("UnpledgeV2: no unpledge_sequence_info row — skip (already unpledged or not pledged via V2)",
					"mainTxID", mainTxID)
				return nil
			}
			if qerr != nil {
				return fmt.Errorf("UnpledgeV2: ownership-check query unpledge_sequence_info for mainTxID %q: %w", mainTxID, qerr)
			}
			// Row exists but belongs to a different quorum — defense-in-depth catch.
			msg := fmt.Sprintf("UnpledgeV2: quorum ownership mismatch — skip mainTxID=%s caller_did=%s row_owner=%s",
				mainTxID, quorumDID, owner)
			c.log.Warn(msg)
			c.writeUnpledgeMismatch(msg)
			return nil
		}
		return fmt.Errorf("UnpledgeV2: query unpledge_sequence_info for mainTxID %q: %w", mainTxID, err)
	}
	if len(pledgeTokens) == 0 {
		c.log.Info("UnpledgeV2: empty pledge_tokens — skip", "mainTxID", mainTxID)
		return nil
	}

	// --- Check all tokens are PLEDGED (or already FREE for partial retry) ----
	currentTokens, err := c.w.ReadTokensByIDs(ctx, pledgeTokens)
	if err != nil {
		return fmt.Errorf("UnpledgeV2: read current token states for mainTxID %q: %w", mainTxID, err)
	}

	allFree := true
	for _, tokenID := range pledgeTokens {
		token, ok := currentTokens[tokenID]
		if !ok {
			return fmt.Errorf("UnpledgeV2: token %q not found in DB (mainTxID=%q)", tokenID, mainTxID)
		}
		status := token.TokenStatus
		if status == int16(constants.TokenStatus_Pledged) {
			allFree = false
		} else if status != int16(constants.TokenStatus_Free) {
			return fmt.Errorf("UnpledgeV2: token %q has unexpected status %d for unpledge (want PLEDGED=6 or FREE=0), mainTxID=%q",
				tokenID, status, mainTxID)
		}
	}

	if allFree {
		// All tokens are already FREE — idempotent success. Clean up the row.
		c.log.Info("UnpledgeV2: all tokens already FREE — removing sequence row", "mainTxID", mainTxID)
		if _, err := c.w.Pool().Exec(ctx,
			`DELETE FROM unpledge_sequence_info WHERE tx_id = $1`, mainTxID,
		); err != nil {
			c.log.Error("UnpledgeV2: failed to delete stale unpledge_sequence_info",
				"mainTxID", mainTxID, "error", err)
			// Non-fatal: the tokens are free; return nil.
		}
		return nil
	}

	// --- Atomic unpledge: UPDATE tokenchain + UPDATE tokens in one tx -----------
	unpledgeRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Unpledge))

	unpledgeTx, err := c.w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("UnpledgeV2: begin unpledge tx: %w", err)
	}
	defer unpledgeTx.Rollback(ctx) //nolint:errcheck

	// Step 1: Insert proofTx in `transaction` table
	// if _, err := unpledgeTx.Exec(ctx, `
	//     INSERT INTO transactions (id, info, signature)
	// 	VALUES ($1, $2, $3)
	// `, proofTx.ID, proofTx.Info, proofTx.Signature); err != nil {
	// 	return fmt.Errorf("UnpledgeV2: insert proofTx into transactions table for prevTX %q: %w", mainTxID, err)
	// }

	// Step 1: Add tokenchain and tokenchain_index entries for unpledge tokens
	rows, err := unpledgeTx.Query(ctx, `
		WITH inserted AS (
			INSERT INTO tokenchain (
				token_id,
				transaction_id,
				previous_transaction_id,
				role,
				position
			)
			SELECT
				tc.token_id,
				$2,
				tc.transaction_id,
				$3,
				tc.position + 1
			FROM (
				SELECT DISTINCT ON (token_id)
					token_id,
					transaction_id,
					position
				FROM tokenchain
				WHERE token_id = ANY($1::text[])
				ORDER BY token_id, position DESC
			) tc
			RETURNING id, token_id
		)
		INSERT INTO tokenchain_index (token_id, index)
		SELECT
			i.token_id,
			ARRAY[i.id]
		FROM inserted i
		ON CONFLICT (token_id)
		DO UPDATE
		SET index = tokenchain_index.index || EXCLUDED.index,
			updated_at = NOW()
		RETURNING token_id;
	`, pledgeTokens, proofTx.ID, unpledgeRoleID)
	if err != nil {
		return fmt.Errorf("UnpledgeV2: update tokenchain role for mainTxID %q: %w", mainTxID, err)
	}
	defer rows.Close()

	updatedCount := 0

	for rows.Next() {
		var tokenID string
		if err := rows.Scan(&tokenID); err != nil {
			return fmt.Errorf("UnpledgeV2: scan failed: %w", err)
		}
		updatedCount++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("UnpledgeV2: rows err: %w", err)
	}

	if updatedCount != len(pledgeTokens) {
		return fmt.Errorf(
			"UnpledgeV2: tokenchain update mismatch (expected %d, got %d). Missing tokens in tokenchain?",
			len(pledgeTokens),
			updatedCount,
		)
	}

	// Step 2: Update tokens status to FREE
	if _, err := unpledgeTx.Exec(ctx, `
		UPDATE tokens SET token_status = $2, latest_role = $3, updated_at = NOW(),
		latest_position = latest_position + 1, transaction_id = $4
		WHERE token_id = ANY($1::text[])
	`, pledgeTokens, int16(constants.TokenStatus_Free), unpledgeRoleID, proofTx.ID); err != nil {
		return fmt.Errorf("UnpledgeV2: update token status for mainTxID %q: %w", mainTxID, err)
	}


	if err := unpledgeTx.Commit(ctx); err != nil {
		return fmt.Errorf("UnpledgeV2: commit unpledge tx for mainTxID %q: %w", mainTxID, err)
	}

	// All tokens succeeded — increment token_denom and delete the sequence row.
	cleanupTx, err := c.w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("UnpledgeV2: begin cleanup tx for mainTxID %q: %w", mainTxID, err)
	}
	defer cleanupTx.Rollback(ctx) //nolint:errcheck

	// Increment token_denom (upsert pattern from pledge.go:657-674).
	tokenDenomMap := make(map[float64]int64, len(pledgeTokens))
	for _, tokenID := range pledgeTokens {
		tokenValue, err := util.GetTokenValueFromTokenID(tokenID)
		if err != nil {
			// Non-fatal: log and skip so we still delete the sequence row.
			c.log.Error("UnpledgeV2: get token value for denom update",
				"tokenID", tokenID, "mainTxID", mainTxID, "error", err)
			continue
		}
		tokenDenomMap[tokenValue]++
	}

	if len(tokenDenomMap) > 0 {
		denomValueList, denomCountList := util.UnzipMap(tokenDenomMap)
		if _, err := cleanupTx.Exec(ctx, `
			INSERT INTO token_denom (did, denom, count)
			SELECT $1, d.denom, d.count
			FROM UNNEST($2::NUMERIC[], $3::BIGINT[]) AS d(denom, count)
			ON CONFLICT (did, denom)
			DO UPDATE SET
				count = token_denom.count + EXCLUDED.count,
				updated_at = NOW()
		`, quorumDID, denomValueList, denomCountList); err != nil {
			return fmt.Errorf("UnpledgeV2: increment token_denom for mainTxID %q: %w", mainTxID, err)
		}
	}

	// Delete unpledge_sequence_info after all tokens are committed.
	if _, err := cleanupTx.Exec(ctx,
		`DELETE FROM unpledge_sequence_info WHERE tx_id = $1`, mainTxID,
	); err != nil {
		return fmt.Errorf("UnpledgeV2: delete unpledge_sequence_info for mainTxID %q: %w", mainTxID, err)
	}

	if err := cleanupTx.Commit(ctx); err != nil {
		return fmt.Errorf("UnpledgeV2: commit cleanup tx for mainTxID %q: %w", mainTxID, err)
	}

	c.log.Info("UnpledgeV2 complete", "mainTxID", mainTxID, "tokens", len(pledgeTokens))

	return nil
}

// Deprecated: unpledgeSingleTokenV2 is unused under the V2 single-transaction
// architecture. UnpledgeV2 now performs an atomic UPDATE on all pledge rows
// instead of per-token INSERT. Retained for reference only.
func (c *Core) unpledgeSingleTokenV2(
	ctx context.Context,
	tokenID string,
	mainTxID string,
	triggerTxID string,
	quorumDID string,
	dc types.DIDCrypto,
	epoch int,
	network string,
) error {
	// Fetch the latest tokenchain row — this should be the pledge row, so
	// latestRow.TransactionID = pledgeTxID.
	latestRows, err := c.w.ReadLatestTokenChainRows(ctx, []string{tokenID})
	if err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: read latest tokenchain for %q: %w", tokenID, err)
	}
	latestRow := latestRows[tokenID]
	if latestRow == nil {
		return fmt.Errorf("unpledgeSingleTokenV2: no tokenchain entry for token %q", tokenID)
	}

	prevTx := latestRow.TransactionID

	tokenValue, err := util.GetTokenValueFromTokenID(tokenID)
	if err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: get token value for %q: %w", tokenID, err)
	}

	// Build per-token unpledgeTxInfo.
	unpledgeTxInfo := &models.TransactionInfo{
		Initiator: quorumDID,
		Owner:     quorumDID,
		Epoch:     epoch,
		Network:   network,
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{{
				TokenID:               tokenID,
				PreviousTransactionID: prevTx,
				TokenValue:            tokenValue,
			}},
		},
		Memo: fmt.Sprintf("UNPLEDGE pledged_for_tx=%s unpledged_by_tx=%s", mainTxID, triggerTxID),
	}

	// Deterministic identity.
	unpledgeTxID, err := util.GetTransactionID(unpledgeTxInfo)
	if err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: compute unpledgeTxID for token %q: %w", tokenID, err)
	}

	// Sign.
	unpledgeSig, err := util.SignTransaction(dc, unpledgeTxInfo)
	if err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: sign unpledge tx %q for token %q: %w", unpledgeTxID, tokenID, err)
	}
	signature := &models.Signature{InitiatorSignature: unpledgeSig}

	// Build payload.
	tokenChainRows, tokenStates, affectedTokens, err := c.w.BuildUnpledgePayload(ctx, unpledgeTxID, unpledgeTxInfo, quorumDID)
	if err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: build unpledge payload for token %q (tx %q): %w", tokenID, unpledgeTxID, err)
	}

	// Persist.
	if err := c.w.PersistUnpledgeV2(ctx, unpledgeTxInfo, signature, unpledgeTxID, quorumDID, tokenChainRows, tokenStates, affectedTokens); err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: persist unpledge tx %q for token %q: %w", unpledgeTxID, tokenID, err)
	}

	return nil
}

// writeUnpledgeMismatch appends a single-line audit record to
// unpledge_mismatch.log in the node config directory. Lazy-initialized on
// first call via sync.Once — most nodes will never hit a mismatch, so we
// avoid opening the file at startup. Writes are serialized via a mutex
// because os.File.Write is not safe for concurrent callers.
//
// This function never returns an error and never panics: audit logging must
// not crash the node. On open failure, the event is still visible via c.log.
func (c *Core) writeUnpledgeMismatch(line string) {
	c.unpledgeAuditLogOnce.Do(func() {
		path := filepath.Join(c.cfg.NodeConfigDir, "unpledge_mismatch.log")
		fp, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			c.log.Error("writeUnpledgeMismatch: failed to open audit log",
				"path", path, "err", err)
			return
		}
		c.unpledgeAuditLog = fp
	})
	if c.unpledgeAuditLog == nil {
		return
	}
	c.unpledgeAuditLogMu.Lock()
	defer c.unpledgeAuditLogMu.Unlock()
	_, _ = c.unpledgeAuditLog.WriteString(
		time.Now().UTC().Format(time.RFC3339Nano) + " " + line + "\n",
	)
}
