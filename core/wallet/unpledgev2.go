package wallet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// UnpledgeV2 processes per-token unpledge transactions for all tokens
// associated with mainTxID in the unpledge_sequence_info table.
//
// Design: each token is unpledged independently — its own TransactionInfo, its
// own txID, its own DB transaction (via PersistUnpledgeV2). This means partial
// failure is tolerated: tokens that have already been committed remain
// committed, and the unpledge_sequence_info row is only deleted when ALL
// tokens succeed (enabling safe retry).
//
// Idempotency: if no unpledge_sequence_info row exists for mainTxID the
// function returns nil immediately (already unpledged or never pledged via V2).
// Tokens already in FREE status are skipped in the per-token loop.
//
// Parameters:
//   - mainTxID:   lookup key in unpledge_sequence_info (NOT the pledge txID)
//   - triggerTxID: ID of the broadcast main-transfer that triggered this
//     unpledge — stored in the Memo field for auditability
//   - quorumDID:  DID that owns the pledged tokens
//   - quorumDC:   DIDCrypto for signing each per-token unpledge transaction
func (w *Wallet) UnpledgeV2(
	ctx context.Context,
	mainTxID string,
	triggerTxID string,
	quorumDID string,
	quorumDC types.DIDCrypto,
) error {
	if mainTxID == "" {
		return fmt.Errorf("UnpledgeV2: mainTxID is required")
	}
	if quorumDID == "" {
		return fmt.Errorf("UnpledgeV2: quorumDID is required")
	}
	if quorumDC == nil {
		return fmt.Errorf("UnpledgeV2: quorumDC (DIDCrypto) is required")
	}

	// --- Idempotency guard: look up unpledge_sequence_info -------------------
	var pledgeTokens []string
	var epoch int
	err := w.db.Pool().QueryRow(ctx,
		`SELECT pledge_tokens, epoch FROM unpledge_sequence_info WHERE tx_id = $1`,
		mainTxID,
	).Scan(&pledgeTokens, &epoch)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Already unpledged or never pledged via V2 — idempotent success.
			w.log.Info("UnpledgeV2: no unpledge_sequence_info row — skip (already unpledged or not pledged via V2)",
				"mainTxID", mainTxID)
			return nil
		}
		return fmt.Errorf("UnpledgeV2: query unpledge_sequence_info for mainTxID %q: %w", mainTxID, err)
	}
	if len(pledgeTokens) == 0 {
		w.log.Info("UnpledgeV2: empty pledge_tokens — skip", "mainTxID", mainTxID)
		return nil
	}

	// --- Check all tokens are PLEDGED (or already FREE for partial retry) ----
	currentTokens, err := w.readTokensByIDs(ctx, pledgeTokens)
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
		w.log.Info("UnpledgeV2: all tokens already FREE — removing sequence row", "mainTxID", mainTxID)
		if _, err := w.db.Pool().Exec(ctx,
			`DELETE FROM unpledge_sequence_info WHERE tx_id = $1`, mainTxID,
		); err != nil {
			w.log.Error("UnpledgeV2: failed to delete stale unpledge_sequence_info",
				"mainTxID", mainTxID, "error", err)
			// Non-fatal: the tokens are free; return nil.
		}
		return nil
	}

	// --- Per-token unpledge loop ---------------------------------------------
	var failedTokens []string
	var succeededTokens []string

	for _, tokenID := range pledgeTokens {
		// Skip tokens already FREE (partial retry — they were committed in a
		// prior attempt).
		if currentTokens[tokenID].TokenStatus == int16(constants.TokenStatus_Free) {
			succeededTokens = append(succeededTokens, tokenID)
			continue
		}

		if err := w.unpledgeSingleTokenV2(ctx, tokenID, mainTxID, triggerTxID, quorumDID, quorumDC, epoch); err != nil {
			w.log.Error("UnpledgeV2: per-token unpledge failed",
				"tokenID", tokenID, "mainTxID", mainTxID, "error", err)
			failedTokens = append(failedTokens, tokenID)
			// Do NOT abort — other tokens may still succeed.
			continue
		}
		succeededTokens = append(succeededTokens, tokenID)
	}

	// --- Post-loop: cleanup only if ALL tokens succeeded ---------------------
	if len(failedTokens) > 0 {
		return fmt.Errorf("UnpledgeV2: %d/%d tokens failed for mainTxID %q: %v",
			len(failedTokens), len(pledgeTokens), mainTxID, failedTokens)
	}

	// All tokens succeeded — increment token_denom and delete the sequence row.
	cleanupTx, err := w.BeginTx(ctx)
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
			w.log.Error("UnpledgeV2: get token value for denom update",
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

	// Delete unpledge_sequence_info ONLY after all tokens committed.
	if _, err := cleanupTx.Exec(ctx,
		`DELETE FROM unpledge_sequence_info WHERE tx_id = $1`, mainTxID,
	); err != nil {
		return fmt.Errorf("UnpledgeV2: delete unpledge_sequence_info for mainTxID %q: %w", mainTxID, err)
	}

	if err := cleanupTx.Commit(ctx); err != nil {
		return fmt.Errorf("UnpledgeV2: commit cleanup tx for mainTxID %q: %w", mainTxID, err)
	}

	w.log.Info("UnpledgeV2 complete", "mainTxID", mainTxID, "tokens", len(pledgeTokens))

	return nil
}

// unpledgeSingleTokenV2 builds and persists an independent unpledge transaction
// for a single token. Each call opens its own DB transaction via PersistUnpledgeV2.
//
// The unpledgeTxID is deterministic: SHA3_256(SerializeTransactionInfo(txInfo)),
// so retrying with the same inputs produces the same result (idempotent via
// ON CONFLICT DO NOTHING in PersistUnpledgeV2).
func (w *Wallet) unpledgeSingleTokenV2(
	ctx context.Context,
	tokenID string,
	mainTxID string,
	triggerTxID string,
	quorumDID string,
	quorumDC types.DIDCrypto,
	epoch int,
) error {
	// Fetch the latest tokenchain row — this should be the pledge row, so
	// latestRow.TransactionID = pledgeTxID.
	latestRows, err := w.readLatestTokenChainRows(ctx, []string{tokenID})
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
		Network:   "", // unpledge is node-local; no network broadcast
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{{
				TokenID:               tokenID,
				PreviousTransactionID: prevTx,
				TokenValue:            tokenValue,
				DID:                   quorumDID,
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
	unpledgeSig, err := util.SignTransaction(quorumDC, unpledgeTxInfo)
	if err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: sign unpledge tx %q for token %q: %w", unpledgeTxID, tokenID, err)
	}
	signature := &models.Signature{InitiatorSignature: unpledgeSig}

	// Build payload.
	tokenChainRows, tokenStates, affectedTokens, err := w.BuildUnpledgePayload(ctx, unpledgeTxID, unpledgeTxInfo, quorumDID)
	if err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: build unpledge payload for token %q (tx %q): %w", tokenID, unpledgeTxID, err)
	}

	// Persist.
	if err := w.PersistUnpledgeV2(ctx, unpledgeTxInfo, signature, unpledgeTxID, quorumDID, tokenChainRows, tokenStates, affectedTokens); err != nil {
		return fmt.Errorf("unpledgeSingleTokenV2: persist unpledge tx %q for token %q: %w", unpledgeTxID, tokenID, err)
	}

	return nil
}
