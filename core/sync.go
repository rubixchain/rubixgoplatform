package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	rubixsync "github.com/rubixchain/rubixgoplatform/core/sync"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
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
	Status  bool                                   `json:"status"`
	Message string                                 `json:"message"`
	Data    map[string][]types.TransactionWithRole `json:"data"`
}

// SyncTransactionChain handles POST /rubix/v1/internal/sync_transaction_chain.
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

	result := make(map[string][]types.TransactionWithRole)
	for _, tokenID := range req.TokenIDs {
		txs, err := c.w.GetTransactionsAndTokenRoleByTokenID(tokenID)
		if err != nil {
			c.log.Warn("SyncTransactionChain: failed to fetch chain", "tokenID", tokenID, "err", err)
			continue
		}
		if len(excludeSet) > 0 {
			filtered := make([]types.TransactionWithRole, 0, len(txs))
			for _, tx := range txs {
				if !excludeSet[tx.Tx.ID] {
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
func (c *Core) SyncTransactionChainsFromPeer(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string, transferNFTOwnership bool, isFullnode bool) error {
	if len(tokenIDs) == 0 {
		c.log.Debug("SyncTransactionChainsFromPeer: No token IDs to sync, returning")
		return nil
	}

	c.log.Debug("SyncTransactionChainsFromPeer: Starting sync",
		"peerDID", peerDID,
		"tokenIDs", tokenIDs,
		"tokenCount", len(tokenIDs),
		"excludeTxIDCount", len(excludeTxIDs),
		"prevTxIDCount", len(prevTxIDs),
		"isFullnode", isFullnode,
	)

	req := syncTxChainRequest{DID: peerDID, TokenIDs: tokenIDs, ExcludeTransactionIDs: excludeTxIDs}

	p, err := c.getPeer(peerDID)
	if err != nil {
		c.log.Error("SyncTransactionChainsFromPeer: getPeer failed", "peerDID", peerDID, "err", err)
		return fmt.Errorf("SyncTransactionChainsFromPeer: getPeer failed: %w", err)
	}
	defer p.Close()

	c.log.Debug("SyncTransactionChainsFromPeer: Peer connection established, sending sync request", "peerDID", peerDID)

	var resp syncTxChainResponse
	if err = p.SendJSONRequest("POST", APISyncTransactionChain, nil, &req, &resp, false, 30*time.Second); err != nil {
		c.log.Error("SyncTransactionChainsFromPeer: Request failed", "peerDID", peerDID, "err", err)
		return fmt.Errorf("SyncTransactionChainsFromPeer: request failed: %w", err)
	}

	if !resp.Status {
		c.log.Error("SyncTransactionChainsFromPeer: Peer returned error", "peerDID", peerDID, "message", resp.Message)
		return fmt.Errorf("SyncTransactionChainsFromPeer: peer returned error: %s", resp.Message)
	}

	c.log.Debug("SyncTransactionChainsFromPeer: Received response from peer",
		"peerDID", peerDID,
		"tokenCount", len(resp.Data),
		"message", resp.Message,
	)

	for tokenID, txs := range resp.Data {
		c.log.Info("SyncTransactionChainsFromPeer: Applying chain for token",
			"tokenID", tokenID,
			"txCount", len(txs),
		)
		prevTxID := prevTxIDs[tokenID] // empty string if not in map — applyTokenChainFromSync handles this
		if isFullnode {
			if err := c.applyTokenChainFromSyncForFullNode(tokenID, txs, prevTxID); err != nil {
				c.log.Warn("SyncTransactionChainsFromPeer: fullnode apply failed (non-fatal)", "tokenID", tokenID, "err", err)
			} else {
				c.log.Info("SyncTransactionChainsFromPeer: Chain applied successfully (fullnode)", "tokenID", tokenID, "txCount", len(txs))
			}
		} else {
			if err := c.applyTokenChainFromSync(tokenID, txs, prevTxID, transferNFTOwnership); err != nil {
				c.log.Warn("SyncTransactionChainsFromPeer: apply failed (non-fatal)", "tokenID", tokenID, "err", err)
			} else {
				c.log.Info("SyncTransactionChainsFromPeer: Chain applied successfully", "tokenID", tokenID, "txCount", len(txs))
			}
		}
	}

	c.log.Info("SyncTransactionChainsFromPeer: Sync completed", "peerDID", peerDID, "tokenCount", len(tokenIDs))
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
func (c *Core) applyTokenChainFromSync(tokenID string, remoteTxs []types.TransactionWithRole, prevTxID string, transferNFTOwnership bool) error {
	if len(remoteTxs) == 0 {
		return nil
	}

	// Step 1: Get local tokenchain (ordered by position ASC) to determine what we already have.
	localChain, err := c.w.GetTokenChainByTokenID(tokenID, false)
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
		role   int16  // Role for this token within this transaction
	}
	enriched := make([]txWithPrev, 0, len(remoteTxs))
	for _, tx := range remoteTxs {
		var txInfo models.TransactionInfo
		var prev string
		if err := json.Unmarshal(tx.Tx.Info, &txInfo); err == nil {
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
				if prev == "" {
					for _, t := range txInfo.Tokens.SmartContract {
						if t != nil && t.TokenID == tokenID {
							prev = t.PreviousTransactionID
							break
						}
					}
				}
			}
		}
		enriched = append(enriched, txWithPrev{tx: tx.Tx, prevID: prev, role: tx.Role})
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
		if localChain[i].TransactionID != remoteTxs[i].Tx.ID {
			c.log.Error("applyTokenChainFromSync: FORK DETECTED — local chain diverges from remote chain",
				"tokenID", tokenID,
				"position", i,
				"localTxID", localChain[i].TransactionID,
				"remoteTxID", remoteTxs[i].Tx.ID,
			)
			return fmt.Errorf("applyTokenChainFromSync: fork detected for token %s at position %d: local=%s remote=%s",
				tokenID, i, localChain[i].TransactionID, remoteTxs[i].Tx.ID)
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
					var errTokenValue error

					tokenType = int16(models.GetTokenTypeID(constants.TokenType_RBT))

					tokenValue, errTokenValue = util.GetTokenValueFromTokenID(tokenID)
					if errTokenValue != nil {
						return fmt.Errorf("applyTokenChainFromSync: failed to get token value for RBT Token %s: %w", tokenID, errTokenValue)
					}
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

		var parentTokenID string
		_, err := util.GetRbtIDElements(tokenID)
		// Only get parent token for RBT token
		if err == nil {
			if tokenValue != rubixmath.OneFloat() {
				parentTokenID, err = util.TokenID(tokenID).GetParentToken()
				if err != nil {
					return fmt.Errorf("applyTokenChainFromSync: failed to get parent token ID for %s: %w", tokenID, err)
				}
			}
		}

		firstRole := rubixsync.FindTokenRoleInTxn(tokenID, &firstTxInfo, transferNFTOwnership)

		// SC tokens have no Owner — use Initiator as the deploying DID.
		ownerDID := firstTxInfo.Owner
		if ownerDID == "" {
			ownerDID = firstTxInfo.Initiator
		}

		// Ensure the owner DID exists in the dids table before inserting
		// the token — tokens.did has a FK to dids.did. The DID may belong
		// to a remote peer never registered locally.
		if ownerDID != "" {
			algoID, algoErr := c.w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
			if algoErr != nil {
				return fmt.Errorf("applyTokenChainFromSync: resolve algo ID for DID %s: %w", ownerDID, algoErr)
			}
			if didErr := c.w.CreateOrUpdateDID(&models.DID{
				DID:    ownerDID,
				Local:  false,
				AlgoID: algoID,
			}); didErr != nil {
				return fmt.Errorf("applyTokenChainFromSync: upsert DID %s: %w", ownerDID, didErr)
			}
		}

		// Derive token status from the role so synced tokens match what the
		// originating node wrote (mirrors transaction_chain.go logic).
		tokenStatus := int16(constants.TokenStatus_Free)
		switch firstRole {
		case int16(models.GetTokenRoleID(constants.TokenRole_Deploy)):
			tokenStatus = int16(constants.TokenStatus_Deployed)
		case int16(models.GetTokenRoleID(constants.TokenRole_Execute)):
			tokenStatus = int16(constants.TokenStatus_Executed)
		}

		newToken := models.Token{
			TokenID: tokenID,
			DID:     ownerDID,
			ParentTokenID: pgtype.Text{
				String: parentTokenID,
				Valid:  true,
			},
			TransactionID:  newTxs[0].tx.ID,
			TokenType:      tokenType,
			TokenValue:     tokenValue,
			TokenStatus:    tokenStatus,
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
			Role:                  e.role,
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
	lastOwnerDID := lastTxInfo.Owner
	if lastOwnerDID == "" {
		lastOwnerDID = lastTxInfo.Initiator
	}
	// Derive final token status from the last chain entry's role so the
	// token row matches what the originating node wrote.
	lastStatus := int16(constants.TokenStatus_Free)
	switch lastEntry.Role {
	case int16(models.GetTokenRoleID(constants.TokenRole_Deploy)):
		lastStatus = int16(constants.TokenStatus_Deployed)
	case int16(models.GetTokenRoleID(constants.TokenRole_Execute)):
		lastStatus = int16(constants.TokenStatus_Executed)
	}
	// For NFT executions, ownership doesn't change
	// in sync with the chain head when a non-owner subscriber executes an NFT.
	if lastEntry.Role == int16(models.GetTokenRoleID(constants.TokenRole_Execute)) &&
		tokenForUpdate.TokenType != int16(models.GetTokenTypeID(constants.TokenType_SmartContract)) {
		// Leave tokenForUpdate.DID at its existing value.
	} else {
		tokenForUpdate.DID = lastOwnerDID
	}
	tokenForUpdate.TransactionID = newTxs[len(newTxs)-1].tx.ID
	tokenForUpdate.LatestPosition = lastEntry.Position
	tokenForUpdate.LatestRole = lastEntry.Role
	tokenForUpdate.TokenStatus = lastStatus
	tokenForUpdate.UpdatedAt = time.Now()

	// NFT value is mutable per execution and per transfer.
	// RBT/FT/SC values are immutable across the chain and stay as set at creation.
	if tokenForUpdate.TokenType == int16(models.GetTokenTypeID(constants.TokenType_NFT)) && lastTxInfo.Tokens != nil {
		for _, t := range lastTxInfo.Tokens.NFT {
			if t != nil && t.TokenID == tokenID {
				tokenForUpdate.TokenValue = t.TokenValue
				break
			}
		}
	}

	if updateErr := c.w.UpdateToken(tokenForUpdate); updateErr != nil {
		return fmt.Errorf("applyTokenChainFromSync: UpdateToken for %s: %w", tokenID, updateErr)
	}

	c.log.Info("applyTokenChainFromSync: applied chain entries",
		"tokenID", tokenID, "count", len(newTxs), "startPos", nextPosition)
	return nil
}

// applyTokenChainFromSyncForFullNode mirrors applyTokenChainFromSync but
// persists into fullnode tables (fullnode_transactions, fullnode_tokenchain,
// fullnode_rbt/ft/nft/smart_contract) instead of the normal tables.
func (c *Core) applyTokenChainFromSyncForFullNode(tokenID string, remoteTxs []types.TransactionWithRole, prevTxID string) error {
	if len(remoteTxs) == 0 {
		return nil
	}
	c.log.Debug("applyTokenChainFromSyncForFullNode: Starting sync",
		"tokenID", tokenID,
		"remoteTxs", len(remoteTxs),
		"prevTxID", prevTxID,
	)

	localChain, err := c.w.GetFullNodeTokenChainByTokenID(tokenID)
	if err != nil {
		localChain = nil
	}

	if prevTxID != "" && len(localChain) > 0 {
		for _, lc := range localChain {
			if lc.TransactionID == prevTxID {
				c.log.Debug("applyTokenChainFromSyncForFullNode: prevTxID already in local chain, skipping sync",
					"tokenID", tokenID, "prevTxID", prevTxID)
				return nil
			}
		}
	}

	type txWithPrev struct {
		tx     models.Transactions
		prevID string
		role   int16
	}
	enriched := make([]txWithPrev, 0, len(remoteTxs))
	for _, tx := range remoteTxs {
		var txInfo models.TransactionInfo
		var prev string
		if err := json.Unmarshal(tx.Tx.Info, &txInfo); err == nil {
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
				if prev == "" {
					for _, t := range txInfo.Tokens.SmartContract {
						if t != nil && t.TokenID == tokenID {
							prev = t.PreviousTransactionID
							break
						}
					}
				}
			}
		}
		enriched = append(enriched, txWithPrev{tx: tx.Tx, prevID: prev, role: tx.Role})
	}

	for i := 1; i < len(enriched); i++ {
		if enriched[i].prevID != "" && enriched[i].prevID != enriched[i-1].tx.ID {
			return fmt.Errorf("applyTokenChainFromSyncForFullNode: canonical order violation at position %d for token %s: prevTxID %s != expected %s",
				i, tokenID, enriched[i].prevID, enriched[i-1].tx.ID)
		}
	}

	for i := 0; i < len(localChain) && i < len(remoteTxs); i++ {
		if localChain[i].TransactionID != remoteTxs[i].Tx.ID {
			return fmt.Errorf("applyTokenChainFromSyncForFullNode: fork detected for token %s at position %d: local=%s remote=%s",
				tokenID, i, localChain[i].TransactionID, remoteTxs[i].Tx.ID)
		}
	}

	newTxs := enriched[len(localChain):]
	if len(newTxs) == 0 {
		return nil
	}

	var nextPosition int64
	if len(localChain) > 0 {
		nextPosition = localChain[len(localChain)-1].Position + 1
	}

	// Determine token type from the first new transaction.
	var tokenType string
	var firstTxInfo models.TransactionInfo
	if err := json.Unmarshal(newTxs[0].tx.Info, &firstTxInfo); err != nil {
		return fmt.Errorf("applyTokenChainFromSyncForFullNode: cannot parse first tx.Info: %w", err)
	}
	if firstTxInfo.Tokens != nil {
		for _, t := range firstTxInfo.Tokens.RBT {
			if t != nil && t.TokenID == tokenID {
				tokenType = constants.TokenType_RBT
				break
			}
		}
		if tokenType == "" {
			for _, t := range firstTxInfo.Tokens.FT {
				if t != nil && t.TokenID == tokenID {
					tokenType = constants.TokenType_FT
					break
				}
			}
		}
		if tokenType == "" {
			for _, t := range firstTxInfo.Tokens.NFT {
				if t != nil && t.TokenID == tokenID {
					tokenType = constants.TokenType_NFT
					break
				}
			}
		}
		if tokenType == "" {
			for _, t := range firstTxInfo.Tokens.SmartContract {
				if t != nil && t.TokenID == tokenID {
					tokenType = constants.TokenType_SmartContract
					break
				}
			}
		}
	}
	//search it in the quorum tokens as well.
	for _, quorum := range firstTxInfo.Quorums {
		for _, t := range quorum.Tokens {
			if t != nil && t.TokenID == tokenID {
				tokenType = constants.TokenType_RBT
				break
			}
		}

	}

	for _, committedToken := range firstTxInfo.CommittedTokens {
		if committedToken != nil && committedToken.TokenID == tokenID {
			tokenType = constants.TokenType_RBT
			break
		}
	}
	//If token is not found in any of the above 4 assets, we can assume that it is a unpledge token for a transaction
	//So we can assign tokenType as RBT for now.
	if tokenType == "" {
		c.log.Debug("applyTokenChainFromSyncForFullNode: token %s not found in any token array —so assigning RBT as tokenType", tokenID)
		tokenType = constants.TokenType_RBT
	}

	// Build tokenchain entries.
	entries := make([]models.TokenChain, 0, len(newTxs))
	var lastRole int16
	for i, e := range newTxs {
		var prevTxIDPtr *string
		if i == 0 {
			if len(localChain) > 0 {
				tailTxID := localChain[len(localChain)-1].TransactionID
				prevTxIDPtr = &tailTxID
			}
		} else {
			prevID := newTxs[i-1].tx.ID
			prevTxIDPtr = &prevID
		}

		entries = append(entries, models.TokenChain{
			TokenID:               tokenID,
			TransactionID:         e.tx.ID,
			PreviousTransactionID: prevTxIDPtr,
			Role:                  e.role,
			Position:              nextPosition + int64(i),
		})
		lastRole = e.role
	}

	// Extract raw transactions for the wallet method.
	rawTxs := make([]models.Transactions, 0, len(newTxs))
	for _, e := range newTxs {
		rawTxs = append(rawTxs, e.tx)
	}

	var lastTxInfo models.TransactionInfo
	_ = json.Unmarshal(newTxs[len(newTxs)-1].tx.Info, &lastTxInfo)

	if err := c.w.PersistFullNodeSyncedTokenChain(c.Ctx, tokenID, rawTxs, entries, &lastTxInfo, tokenType, lastRole); err != nil {
		return fmt.Errorf("applyTokenChainFromSyncForFullNode: %w", err)
	}

	c.log.Info("applyTokenChainFromSyncForFullNode: applied chain entries",
		"tokenID", tokenID, "count", len(newTxs), "startPos", nextPosition)
	return nil
}
