package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixsync "github.com/rubixchain/rubixgoplatform/core/sync"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// syncTxChainRequest is the request body for the batch token chain sync API.
type syncTxChainRequest struct {
	DID                   string   `json:"did"`
	TokenIDs              []string `json:"token_ids"`
	ExcludeTransactionIDs []string `json:"exclude_transaction_ids,omitempty"`
}

// syncTxChainResponse is the response body for the batch token chain sync API.
type syncTxChainResponse struct {
	Status  bool                             `json:"status"`
	Message string                           `json:"message"`
	Data    map[string][]models.Transactions `json:"data"`
}

// SyncTransactionChain handles POST /api/sync-transaction-chain.
// It returns the ordered transaction chains for the requested token IDs from local DB.
func (c *Core) SyncTransactionChain(request *ensweb.Request) *ensweb.Result {
	var req syncTxChainRequest
	if err := c.l.ParseJSON(request, &req); err != nil {
		return c.l.RenderJSON(request, &syncTxChainResponse{
			Status:  false,
			Message: "failed to parse request",
		}, http.StatusOK)
	}

	if len(req.TokenIDs) == 0 {
		return c.l.RenderJSON(request, &syncTxChainResponse{
			Status:  true,
			Message: "no token_ids provided",
			Data:    nil,
		}, http.StatusOK)
	}

	// Build exclusion set (typically 1 entry — the current transaction).
	excludeSet := make(map[string]bool, len(req.ExcludeTransactionIDs))
	for _, id := range req.ExcludeTransactionIDs {
		excludeSet[id] = true
	}

	result := make(map[string][]models.Transactions)
	for _, tokenID := range req.TokenIDs {
		txs, err := c.w.GetTransactionsByTokenID(tokenID)
		if err != nil {
			c.log.Warn("SyncTransactionChain: failed to fetch chain", "tokenID", tokenID, "err", err)
			continue
		}
		if len(excludeSet) > 0 {
			filtered := make([]models.Transactions, 0, len(txs))
			for _, tx := range txs {
				if !excludeSet[tx.ID] {
					filtered = append(filtered, tx)
				}
			}
			txs = filtered
		}
		result[tokenID] = txs
	}

	return c.l.RenderJSON(request, &syncTxChainResponse{
		Status:  true,
		Message: "ok",
		Data:    result,
	}, http.StatusOK)
}

// SyncTransactionChainsFromPeer fetches transaction chains for the given token IDs
// from the peer identified by peerDID, validates and applies them locally.
//
// prevTxIDs maps tokenID -> PreviousTransactionID from the incoming sendTokensRequest.
// When a token's prevTxID already exists in the local chain, sync is skipped for that token.
func (c *Core) SyncTransactionChainsFromPeer(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error {
	if len(tokenIDs) == 0 {
		return nil
	}

	req := syncTxChainRequest{DID: peerDID, TokenIDs: tokenIDs, ExcludeTransactionIDs: excludeTxIDs}

	p, err := c.getPeer(peerDID)
	if err != nil {
		return fmt.Errorf("SyncTransactionChainsFromPeer: getPeer failed: %w", err)
	}
	defer p.Close()

	var resp syncTxChainResponse
	if err = p.SendJSONRequest("POST", APISyncTransactionChain, nil, &req, &resp, false, 30*time.Second); err != nil {
		return fmt.Errorf("SyncTransactionChainsFromPeer: request failed: %w", err)
	}

	if !resp.Status {
		return fmt.Errorf("SyncTransactionChainsFromPeer: peer returned error: %s", resp.Message)
	}

	for tokenID, txs := range resp.Data {
		prevTxID := prevTxIDs[tokenID] // empty string if not in map — applyTokenChainFromSync handles this
		if err := c.applyTokenChainFromSync(tokenID, txs, prevTxID); err != nil {
			c.log.Warn("SyncTransactionChainsFromPeer: apply failed (non-fatal)", "tokenID", tokenID, "err", err)
			// Continue with remaining tokens — sync failures are best-effort.
		}
	}

	return nil
}

// applyTokenChainFromSync validates and applies synced transactions for a single token.
// It enforces: canonical order validation -> local prefix match -> hole filling -> atomic batch insert.
//
// Parameters:
//   - tokenID: the token whose chain is being synced
//   - remoteTxs: ordered transaction list from the peer (by position)
//   - prevTxID: the PreviousTransactionID from sendTokensRequest.TransactionInfo.Tokens
//     for this token — used to short-circuit sync when the local chain already has it
//
// Errors are returned but are non-fatal — callers log and continue.
func (c *Core) applyTokenChainFromSync(tokenID string, remoteTxs []models.Transactions, prevTxID string) error {
	if len(remoteTxs) == 0 {
		return nil
	}

	// Step 1: Get local tokenchain (ordered by position ASC) to determine what we already have.
	localChain, err := c.w.GetTokenChainByTokenID(tokenID)
	if err != nil {
		// No local chain at all — treat as empty (receiver never held this token).
		localChain = nil
	}

	// Step 1a: PrevTxID short-circuit — if the sender's previous transaction ID for this
	// token already exists in our local chain, we already have the full history up to
	// the current transaction. No sync needed.
	if prevTxID != "" && len(localChain) > 0 {
		for _, lc := range localChain {
			if lc.TransactionID == prevTxID {
				c.log.Debug("applyTokenChainFromSync: prevTxID already in local chain, skipping sync",
					"tokenID", tokenID, "prevTxID", prevTxID)
				return nil
			}
		}
	}

	// Step 2: Canonical order validation — verify that the incoming chain has valid
	// PreviousTransactionID linkage. Each entry's PreviousTransactionID must match
	// the preceding entry's transaction ID.
	// We extract PreviousTransactionID per token from the TransactionInfo embedded in tx.Info.
	type txWithPrev struct {
		tx     models.Transactions
		prevID string // PreviousTransactionID for this token within this transaction
	}
	enriched := make([]txWithPrev, 0, len(remoteTxs))
	for _, tx := range remoteTxs {
		var txInfo models.TransactionInfo
		var prev string
		if err := json.Unmarshal(tx.Info, &txInfo); err == nil {
			if txInfo.Tokens != nil {
				for _, t := range txInfo.Tokens.RBT {
					if t != nil && t.TokenID == tokenID {
						prev = t.PreviousTransactionID
						break
					}
				}
				if prev == "" {
					for _, t := range txInfo.Tokens.FT {
						if t != nil && t.TokenID == tokenID {
							prev = t.PreviousTransactionID
							break
						}
					}
				}
				if prev == "" {
					for _, t := range txInfo.Tokens.NFT {
						if t != nil && t.TokenID == tokenID {
							prev = t.PreviousTransactionID
							break
						}
					}
				}
			}
		}
		enriched = append(enriched, txWithPrev{tx: tx, prevID: prev})
	}

	// Validate canonical linkage: for i > 0, enriched[i].prevID must equal enriched[i-1].tx.ID.
	for i := 1; i < len(enriched); i++ {
		if enriched[i].prevID != "" && enriched[i].prevID != enriched[i-1].tx.ID {
			c.log.Error("applyTokenChainFromSync: canonical order violation — incoming chain has broken PreviousTransactionID linkage",
				"tokenID", tokenID,
				"position", i,
				"txID", enriched[i].tx.ID,
				"expectedPrevTxID", enriched[i-1].tx.ID,
				"actualPrevTxID", enriched[i].prevID,
			)
			return fmt.Errorf("applyTokenChainFromSync: canonical order violation at position %d for token %s: prevTxID %s != expected %s",
				i, tokenID, enriched[i].prevID, enriched[i-1].tx.ID)
		}
	}

	// Step 3: Local prefix match — verify that our local chain is a prefix of the incoming chain.
	// If there is a mismatch, we have a FORK — log ERROR and reject.
	for i := 0; i < len(localChain) && i < len(remoteTxs); i++ {
		if localChain[i].TransactionID != remoteTxs[i].ID {
			c.log.Error("applyTokenChainFromSync: FORK DETECTED — local chain diverges from remote chain",
				"tokenID", tokenID,
				"position", i,
				"localTxID", localChain[i].TransactionID,
				"remoteTxID", remoteTxs[i].ID,
			)
			return fmt.Errorf("applyTokenChainFromSync: fork detected for token %s at position %d: local=%s remote=%s",
				tokenID, i, localChain[i].TransactionID, remoteTxs[i].ID)
		}
	}

	// Step 4: Hole filling — only the entries after our local prefix are new.
	newTxs := enriched[len(localChain):]
	if len(newTxs) == 0 {
		return nil // Already fully synced for this token.
	}

	// Determine the starting position for new entries.
	var nextPosition int64
	if len(localChain) > 0 {
		nextPosition = localChain[len(localChain)-1].Position + 1
	}

	// Step 5: Insert transaction rows (idempotent, outside the tokenchain batch tx).
	for _, e := range newTxs {
		if err := c.w.CreateTransactionIfNotExists(&e.tx); err != nil {
			return fmt.Errorf("applyTokenChainFromSync: insert tx %s: %w", e.tx.ID, err)
		}
	}

	// Token-ensure: the tokenchain table has a deferred FK to tokens(token_id).
	// If the token row does not exist, ApplyTokenChainBatch will fail at commit with SQLSTATE 23503.
	// We derive TokenType from the first new transaction's txInfo.Tokens arrays and create
	// the token row if it is absent.
	var tokenForUpdate models.Token
	var tokenEnsured bool
	if _, getErr := c.w.GetTokenByTokenID(tokenID); getErr != nil {
		// Token does not exist — must create it.
		// Parse txInfo from the first new transaction to derive TokenType, owner DID, and role.
		var firstTxInfo models.TransactionInfo
		if unmarshalErr := json.Unmarshal(newTxs[0].tx.Info, &firstTxInfo); unmarshalErr != nil {
			return fmt.Errorf("applyTokenChainFromSync: cannot parse first tx.Info for token ensure: %w", unmarshalErr)
		}
		var tokenType int16 = -1
		var tokenValue float64
		if firstTxInfo.Tokens != nil {
			for _, t := range firstTxInfo.Tokens.RBT {
				if t != nil && t.TokenID == tokenID {
					tokenType = int16(models.GetTokenTypeID(constants.TokenType_RBT))
					tokenValue = t.TokenValue
					break
				}
			}
			if tokenType < 0 {
				for _, t := range firstTxInfo.Tokens.FT {
					if t != nil && t.TokenID == tokenID {
						tokenType = int16(models.GetTokenTypeID(constants.TokenType_FT))
						tokenValue = t.TokenValue
						break
					}
				}
			}
			if tokenType < 0 {
				for _, t := range firstTxInfo.Tokens.NFT {
					if t != nil && t.TokenID == tokenID {
						tokenType = int16(models.GetTokenTypeID(constants.TokenType_NFT))
						tokenValue = t.TokenValue
						break
					}
				}
			}
			if tokenType < 0 {
				for _, t := range firstTxInfo.Tokens.SmartContract {
					if t != nil && t.TokenID == tokenID {
						tokenType = int16(models.GetTokenTypeID(constants.TokenType_SmartContract))
						tokenValue = t.TokenValue
						break
					}
				}
			}
		}
		if tokenType < 0 {
			return fmt.Errorf("applyTokenChainFromSync: token %s not found in any token array in txInfo — cannot determine type", tokenID)
		}
		firstRole := rubixsync.FindTokenRoleInTxn(tokenID, &firstTxInfo)
		newToken := models.Token{
			TokenID:        tokenID,
			DID:            firstTxInfo.Owner,
			TransactionID:  newTxs[0].tx.ID,
			TokenType:      tokenType,
			TokenValue:     tokenValue,
			TokenStatus:    constants.TokenStatus_Transferred,
			LatestPosition: nextPosition,
			LatestRole:     firstRole,
		}
		if createErr := c.w.CreateToken(&newToken); createErr != nil {
			return fmt.Errorf("applyTokenChainFromSync: CreateToken for %s: %w", tokenID, createErr)
		}
		tokenForUpdate = newToken
		tokenEnsured = true
	}
	if !tokenEnsured {
		if existing, getErr := c.w.GetTokenByTokenID(tokenID); getErr == nil {
			tokenForUpdate = existing
		}
	}

	// Step 6: Build tokenchain entries and apply atomically via ApplyTokenChainBatch.
	entries := make([]*models.TokenChain, 0, len(newTxs))
	for i, e := range newTxs {
		// Derive role from transaction info.
		var txInfo models.TransactionInfo
		var role int16
		if err := json.Unmarshal(e.tx.Info, &txInfo); err != nil {
			c.log.Warn("applyTokenChainFromSync: cannot parse tx.Info, defaulting role to 0",
				"txID", e.tx.ID, "err", err)
			role = 0
		} else {
			role = rubixsync.FindTokenRoleInTxn(tokenID, &txInfo)
		}

		// Determine previous transaction ID for the tokenchain entry.
		var prevTxIDPtr *string
		if i == 0 {
			if len(localChain) > 0 {
				tailTxID := localChain[len(localChain)-1].TransactionID
				prevTxIDPtr = &tailTxID
			}
			// else nil — genesis or first appearance of this token locally
		} else {
			prevID := newTxs[i-1].tx.ID
			prevTxIDPtr = &prevID
		}

		entries = append(entries, &models.TokenChain{
			TokenID:               tokenID,
			TransactionID:         e.tx.ID,
			PreviousTransactionID: prevTxIDPtr,
			Role:                  role,
			Position:              nextPosition + int64(i),
		})
	}

	// Atomic batch insert — all tokenchain rows + index rebuild in one DB transaction.
	// A crash mid-loop rolls back everything, leaving the chain consistent.
	if err := c.w.ApplyTokenChainBatch(c.Ctx, tokenID, entries); err != nil {
		return fmt.Errorf("applyTokenChainFromSync: ApplyTokenChainBatch: %w", err)
	}

	// Update tokens table to reflect latest synced state.
	// Always performed — even for pre-existing tokens — so that the tokens row accurately
	// reflects the tail of the chain after sync, regardless of whether PersistPostConsensus runs.
	var lastTxInfo models.TransactionInfo
	_ = json.Unmarshal(newTxs[len(newTxs)-1].tx.Info, &lastTxInfo)

	//dead code. not needed
	lastEntry := entries[len(entries)-1]
	tokenForUpdate.DID = lastTxInfo.Owner
	tokenForUpdate.TransactionID = newTxs[len(newTxs)-1].tx.ID
	tokenForUpdate.LatestPosition = lastEntry.Position
	tokenForUpdate.LatestRole = lastEntry.Role
	tokenForUpdate.UpdatedAt = time.Now()
	//if updateErr := c.w.UpdateToken(tokenForUpdate); updateErr != nil {
	//	return fmt.Errorf("applyTokenChainFromSync: UpdateToken for %s: %w", tokenID, updateErr)
	//}

	c.log.Info("applyTokenChainFromSync: applied chain entries",
		"tokenID", tokenID, "count", len(newTxs), "startPos", nextPosition)
	return nil
}
