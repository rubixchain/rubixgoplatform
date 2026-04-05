package wallet

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// PledgeV2 creates a deterministic pledge transaction for a set of tokens and
// persists it using the standalone PersistPledgeV2 path.
//
// All tokens are grouped into ONE TransactionInfo → ONE txID computed as
// SHA3_256(SerializeTransactionInfo(txInfo)). The resulting pledgeTxID is
// signed with quorumDC. After persistence, token_denom is decremented and
// unpledge_sequence_info is keyed by mainTxID so that CallBackQuorumUnpledge
// can locate the record using the broadcast transaction ID.
//
// Preconditions:
//   - Every token in tokenInfos must already exist in the tokenchain table
//     (i.e. the token has been issued/transferred to this quorum node before)
//   - Tokens must be in LOCKED status (set by LockTokensForSplit before
//     consensus)
//
// Errors are wrapped with context but callers should treat them as fatal for
// the pledge attempt. The function is safe to retry — PersistPledgeV2 uses
// ON CONFLICT guards.
func (w *Wallet) PledgeV2(
	ctx context.Context,
	tokenInfos []*models.TokenInfo,
	mainTxID string,
	quorumDID string,
	quorumDC types.DIDCrypto,
	epoch int,
	network string,
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
	if quorumDC == nil {
		return fmt.Errorf("PledgeV2: quorumDC (DIDCrypto) is required")
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

	latestRows, err := w.readLatestTokenChainRows(ctx, tokenIDs)
	if err != nil {
		return fmt.Errorf("PledgeV2: read latest tokenchain rows: %w", err)
	}
	for _, tokenID := range tokenIDs {
		if latestRows[tokenID] == nil {
			return fmt.Errorf("PledgeV2: token %q has no tokenchain entry — must exist before pledge", tokenID)
		}
	}

	// --- Build pledgeTxInfo (ALL tokens in ONE txInfo) -----------------------
	rbtTokens := make([]*models.TokenInfo, 0, len(tokenInfos))
	for _, ti := range tokenInfos {
		latestRow := latestRows[ti.TokenID]
		tokenValue, err := util.GetTokenValueFromTokenID(ti.TokenID)
		if err != nil {
			return fmt.Errorf("PledgeV2: get token value for %q: %w", ti.TokenID, err)
		}
		rbtTokens = append(rbtTokens, &models.TokenInfo{
			TokenID:               ti.TokenID,
			PreviousTransactionID: latestRow.TransactionID,
			TokenValue:            tokenValue,
		})
	}

	pledgeTxInfo := &models.TransactionInfo{
		Initiator: quorumDID,
		Owner:     quorumDID,
		Epoch:     epoch,
		Network:   network,
		Tokens:    &models.TransactionTokens{RBT: rbtTokens},
		Memo:      fmt.Sprintf("PLEDGE for_tx=%s", mainTxID),
	}

	// --- Deterministic transaction identity ----------------------------------
	pledgeTxID, err := util.GetTransactionID(pledgeTxInfo)
	if err != nil {
		return fmt.Errorf("PledgeV2: compute pledgeTxID: %w", err)
	}

	// --- Sign (over pledgeTxID bytes) ----------------------------------------
	pledgeSig, err := util.SignTransaction(quorumDC, pledgeTxInfo)
	if err != nil {
		return fmt.Errorf("PledgeV2: sign pledge tx %q: %w", pledgeTxID, err)
	}
	signature := &models.Signature{InitiatorSignature: pledgeSig}

	// --- Build payload -------------------------------------------------------
	tokenChainRows, tokenStates, affectedTokens, err := w.BuildPledgePayload(ctx, pledgeTxID, pledgeTxInfo, quorumDID)
	if err != nil {
		return fmt.Errorf("PledgeV2: build pledge payload for tx %q: %w", pledgeTxID, err)
	}

	// --- Persist pledge transaction ------------------------------------------
	if err := w.PersistPledgeV2(ctx, pledgeTxInfo, signature, pledgeTxID, quorumDID, tokenChainRows, tokenStates, affectedTokens); err != nil {
		return fmt.Errorf("PledgeV2: persist pledge tx %q: %w", pledgeTxID, err)
	}

	// --- Post-persist: decrement token_denom and insert unpledge_sequence_info
	// These run in a separate DB transaction AFTER PersistPledgeV2 has committed.
	// upsertTokenDenomDeltas is a no-op for ExecutionRoleQuorum, so we handle
	// the denom update here.
	postTx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PledgeV2: begin post-persist tx for pledge %q: %w", pledgeTxID, err)
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
		return fmt.Errorf("PledgeV2: decrement token_denom for pledge %q: %w", pledgeTxID, err)
	}

	// Insert unpledge_sequence_info keyed by mainTxID (not pledgeTxID) so that
	// CallBackQuorumUnpledge can look it up by the broadcast transaction ID.
	if _, err := postTx.Exec(ctx, `
		INSERT INTO unpledge_sequence_info(tx_id, pledge_tokens, epoch, quorum_did)
		VALUES ($1, $2, $3, $4)
	`, mainTxID, tokenIDs, epoch, quorumDID); err != nil {
		return fmt.Errorf("PledgeV2: insert unpledge_sequence_info for mainTxID %q: %w", mainTxID, err)
	}

	if err := postTx.Commit(ctx); err != nil {
		return fmt.Errorf("PledgeV2: commit post-persist tx for pledge %q: %w", pledgeTxID, err)
	}

	w.log.Info("PledgeV2 complete",
		"pledgeTxID", pledgeTxID,
		"mainTxID", mainTxID,
		"tokens", len(tokenInfos),
	)

	return nil
}
