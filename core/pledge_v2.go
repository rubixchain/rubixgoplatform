package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// PledgeV2 records tokenchain entries for a set of pledged tokens, keyed by
// mainTxID. Pledge is a pure DB bookkeeping operation.
//
// Steps performed atomically inside pledgeTx:
//
//	0. INSERT transactions row (id = mainTxID, info = serialized txnInfo,
//	   signature = combined initiator+quorum) so that tokenchain rows
//	   (transaction_id = mainTxID) have a backing transactions.id.
//	1. INSERT tokenchain rows (one per token, transaction_id = mainTxID, role = 8)
//	2. UPDATE tokens (status = PLEDGED, latest_position, latest_role = 8)
//	3. Rebuild tokenchain_index for affected tokens
//
// Post-commit (separate postTx):
//   - Decrement token_denom for quorumDID
//   - INSERT unpledge_sequence_info keyed by mainTxID
//
// Preconditions:
//   - Every token in tokenInfos must already exist in the tokenchain table
//     (i.e. the token has been issued/transferred to this quorum node before)
//
// Errors are wrapped with context but callers should treat them as fatal for
// the pledge attempt. ON CONFLICT (token_id, position) DO NOTHING makes the
// operation idempotent.
func (c *Core) PledgeV2(
	ctx context.Context,
	tokenInfos []*models.TokenInfo,
	mainTxID string,
	quorumDID string,
	epoch int,
	network string,
	txnInfo *models.TransactionInfo,
	initiatorSignature string,
	quorumSignature string,
) error {
	// --- Validation ----------------------------------------------------------
	if len(tokenInfos) == 0 {
		return fmt.Errorf("PledgeV2: no tokens provided")
	}
	if mainTxID == "" {
		return fmt.Errorf("PledgeV2: mainTxID is required")
	}
	if quorumDID == "" {
		return fmt.Errorf("PledgeV2: quorumDID is required")
	}

	// Reject duplicate token IDs.
	seen := make(map[string]struct{}, len(tokenInfos))
	for _, ti := range tokenInfos {
		if ti == nil {
			return fmt.Errorf("PledgeV2: nil TokenInfo in tokenInfos")
		}
		if _, exists := seen[ti.TokenID]; exists {
			return fmt.Errorf("PledgeV2: duplicate token ID %q", ti.TokenID)
		}
		seen[ti.TokenID] = struct{}{}
	}

	// --- Fetch per-token previous transaction IDs ----------------------------
	tokenIDs := make([]string, 0, len(tokenInfos))
	for _, ti := range tokenInfos {
		tokenIDs = append(tokenIDs, ti.TokenID)
	}

	latestRows, err := c.w.ReadLatestTokenChainRows(ctx, tokenIDs)
	if err != nil {
		return fmt.Errorf("PledgeV2: read latest tokenchain rows: %w", err)
	}
	for _, tokenID := range tokenIDs {
		if latestRows[tokenID] == nil {
			return fmt.Errorf("PledgeV2: token %q has no tokenchain entry — must exist before pledge", tokenID)
		}
	}

	// --- Inline pledge persistence (no txID, no signature, no transaction record) ---
	pledgeRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Pledge))

	pledgeTx, err := c.w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PledgeV2: begin pledge tx: %w", err)
	}
	defer pledgeTx.Rollback(ctx) //nolint:errcheck

	// Step 0: INSERT transactions row for mainTxID so that tokenchain rows
	// (transaction_id = mainTxID) have a backing transactions.id. Both info
	// and signature are NOT NULL columns. The full combined signature is used
	// to match what PersistPostConsensus stores on initiator/receiver (byte-equality).
	infoBytes, err := models.SerializeTransactionInfo(txnInfo)
	if err != nil {
		return fmt.Errorf("PledgeV2: serialize txnInfo for transactions insert: %w", err)
	}
	sig := models.Signature{
		InitiatorSignature: initiatorSignature,
		Quorums:            []models.QuorumSignature{{Did: quorumDID, Signature: quorumSignature}},
	}
	sigBytes, err := json.Marshal(&sig)
	if err != nil {
		return fmt.Errorf("PledgeV2: marshal signature for transactions insert: %w", err)
	}
	if _, err := pledgeTx.Exec(ctx, `
		INSERT INTO transactions (id, info, signature, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, mainTxID, infoBytes, sigBytes); err != nil {
		return fmt.Errorf("PledgeV2: insert transactions row for mainTxID %q: %w", mainTxID, err)
	}

	// Step 1: INSERT tokenchain rows (one per token, keyed by mainTxID)
	for _, ti := range tokenInfos {
		latestRow := latestRows[ti.TokenID]
		prevTxID := latestRow.TransactionID
		if _, err := pledgeTx.Exec(ctx, `
			INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			ON CONFLICT (token_id, position) DO NOTHING
		`, ti.TokenID, mainTxID, &prevTxID, pledgeRoleID, latestRow.Position+1); err != nil {
			return fmt.Errorf("PledgeV2: insert tokenchain for token %q: %w", ti.TokenID, err)
		}
	}

	// Step 2: UPDATE tokens (PLEDGED status, new position/role).
	// transaction_id is NOT updated here — the tokens.transaction_id_fk constraint
	// references transactions.id and updating it here is unnecessary; it is set
	// by PersistPostConsensus once the full transfer completes.
	for _, ti := range tokenInfos {
		latestRow := latestRows[ti.TokenID]
		if _, err := pledgeTx.Exec(ctx, `
			UPDATE tokens
			SET token_status = $2, latest_position = $3, latest_role = $4, updated_at = NOW()
			WHERE token_id = $1
		`, ti.TokenID, int16(constants.TokenStatus_Pledged), latestRow.Position+1, pledgeRoleID); err != nil {
			return fmt.Errorf("PledgeV2: update tokens for token %q: %w", ti.TokenID, err)
		}
	}

	// Step 3: Rebuild tokenchain_index within the same pledge tx (atomicity)
	indexRows, err := pledgeTx.Query(ctx, `
		SELECT token_id, array_agg(id ORDER BY position)
		FROM tokenchain
		WHERE token_id = ANY($1::text[])
		GROUP BY token_id
	`, tokenIDs)
	if err != nil {
		return fmt.Errorf("PledgeV2: read tokenchain for index rebuild: %w", err)
	}
	defer indexRows.Close()

	type tokenIndexRow struct {
		tokenID string
		index   []int32
	}
	idxEntries := make([]tokenIndexRow, 0, len(tokenIDs))
	for indexRows.Next() {
		var entry tokenIndexRow
		if err := indexRows.Scan(&entry.tokenID, &entry.index); err != nil {
			return fmt.Errorf("PledgeV2: scan tokenchain index: %w", err)
		}
		idxEntries = append(idxEntries, entry)
	}
	if err := indexRows.Err(); err != nil {
		return fmt.Errorf("PledgeV2: stream tokenchain index: %w", err)
	}
	indexRows.Close()

	if len(idxEntries) > 0 {
		placeholders := make([]string, 0, len(idxEntries))
		args := make([]any, 0, len(idxEntries)*2)
		for i, entry := range idxEntries {
			offset := i*2 + 1
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, NOW(), NOW())", offset, offset+1))
			args = append(args, entry.tokenID, entry.index)
		}
		indexQuery := `
			INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
			VALUES ` + strings.Join(placeholders, ",") + `
			ON CONFLICT (token_id) DO UPDATE SET
				index = EXCLUDED.index,
				updated_at = NOW()
		`
		if _, err := pledgeTx.Exec(ctx, indexQuery, args...); err != nil {
			return fmt.Errorf("PledgeV2: upsert tokenchain_index: %w", err)
		}
	}

	if err := pledgeTx.Commit(ctx); err != nil {
		return fmt.Errorf("PledgeV2: commit pledge tx: %w", err)
	}

	// --- Post-commit: decrement token_denom and insert unpledge_sequence_info
	// These run in a separate DB transaction AFTER pledgeTx has committed.
	postTx, err := c.w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PledgeV2: begin post-persist tx for mainTxID %q: %w", mainTxID, err)
	}
	defer postTx.Rollback(ctx) //nolint:errcheck

	// Build token_denom decrement map.
	tokenDenomMap := make(map[float64]int64, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		tokenValue, err := util.GetTokenValueFromTokenID(tokenID)
		if err != nil {
			return fmt.Errorf("PledgeV2: get token value for denom update of %q: %w", tokenID, err)
		}
		tokenDenomMap[tokenValue]++
	}
	denomValueList, denomCountList := util.UnzipMap(tokenDenomMap)

	if _, err := postTx.Exec(ctx, `
		UPDATE token_denom t
		SET
			count = t.count - d.count,
			updated_at = NOW()
		FROM UNNEST($2::NUMERIC[], $3::BIGINT[]) AS d(denom, count)
		WHERE t.did = $1 AND t.denom = d.denom
	`, quorumDID, denomValueList, denomCountList); err != nil {
		return fmt.Errorf("PledgeV2: decrement token_denom for mainTxID %q: %w", mainTxID, err)
	}

	// Insert unpledge_sequence_info keyed by mainTxID so that
	// CallBackQuorumUnpledge can look it up by the broadcast transaction ID.
	if _, err := postTx.Exec(ctx, `
		INSERT INTO unpledge_sequence_info(tx_id, pledge_tokens, epoch, quorum_did)
		VALUES ($1, $2, $3, $4)
	`, mainTxID, tokenIDs, epoch, quorumDID); err != nil {
		return fmt.Errorf("PledgeV2: insert unpledge_sequence_info for mainTxID %q: %w", mainTxID, err)
	}

	if err := postTx.Commit(ctx); err != nil {
		return fmt.Errorf("PledgeV2: commit post-persist tx for mainTxID %q: %w", mainTxID, err)
	}

	c.log.Info("PledgeV2 complete",
		"mainTxID", mainTxID,
		"tokens", len(tokenInfos),
	)

	return nil
}
