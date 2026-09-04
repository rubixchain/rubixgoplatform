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
	"github.com/rubixchain/rubixgoplatform/setup"
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

// fetchGenesisTxnRequest is the request body for the genesis-only fetch API.
type fetchGenesisTxnRequest struct {
	DID     string `json:"did"`
	TokenID string `json:"token_id"`
}

// fetchGenesisTxnResponse is the response body for the genesis-only fetch API.
type fetchGenesisTxnResponse struct {
	Status  bool                 `json:"status"`
	Message string               `json:"message"`
	Tx      *models.Transactions `json:"tx,omitempty"`
}

// FetchGenesisTransaction handles POST /rubix/v1/internal/fetch_genesis_transaction.
// Unlike SyncTransactionChain, it returns ONLY the genesis (position 0) transaction
// for the requested token ID from local DB — never the rest of the chain. This exists
// for verification-only callers (IsParentTokenBurnt, ValidateMinterAllowlist) that need
// to inspect a token's minting transaction without any risk of a caller then persisting
// newer, not-yet-validated chain activity that happened to be included in a broader
// chain-sync response.
func (c *Core) FetchGenesisTransaction(request *ensweb.Request) *ensweb.Result {
	var req fetchGenesisTxnRequest
	if err := c.l.ParseJSON(request, &req); err != nil {
		return c.l.RenderJSON(request, &fetchGenesisTxnResponse{
			Status:  false,
			Message: "failed to parse request",
		}, http.StatusOK)
	}

	if req.TokenID == "" {
		return c.l.RenderJSON(request, &fetchGenesisTxnResponse{
			Status:  false,
			Message: "token_id is required",
		}, http.StatusOK)
	}

	tx, _, err := c.w.GetTransactionAndRoleAtHeight(req.TokenID, 0)
	if err != nil {
		return c.l.RenderJSON(request, &fetchGenesisTxnResponse{
			Status:  false,
			Message: err.Error(),
		}, http.StatusOK)
	}

	return c.l.RenderJSON(request, &fetchGenesisTxnResponse{
		Status:  true,
		Message: "ok",
		Tx:      tx,
	}, http.StatusOK)
}

// FetchGenesisTransactionFromPeer fetches ONLY the genesis (position 0) transaction
// for tokenID from the peer identified by peerDID. Unlike SyncTransactionChainsFromPeer,
// it never writes anything to local storage — callers that need to verify a fact about
// a token's minting transaction should use this instead of the full chain-sync path, so
// there is no risk of ingesting and persisting a sibling transaction that is still being
// validated elsewhere.
func (c *Core) FetchGenesisTransactionFromPeer(peerDID, tokenID string) (*models.Transactions, error) {
	req := fetchGenesisTxnRequest{DID: peerDID, TokenID: tokenID}

	p, err := c.getPeer(peerDID)
	if err != nil {
		return nil, fmt.Errorf("FetchGenesisTransactionFromPeer: getPeer failed: %w", err)
	}
	defer p.Close()

	var resp fetchGenesisTxnResponse
	if err := p.SendJSONRequest("POST", APIFetchGenesisTxn, nil, &req, &resp, false, 30*time.Second); err != nil {
		return nil, fmt.Errorf("FetchGenesisTransactionFromPeer: request failed: %w", err)
	}

	if !resp.Status || resp.Tx == nil {
		return nil, fmt.Errorf("FetchGenesisTransactionFromPeer: peer returned error: %s", resp.Message)
	}

	return resp.Tx, nil
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
		"excludeTxIDs", excludeTxIDs,
		"prevTxIDs", prevTxIDs,
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

	respSummary := make(map[string]int, len(resp.Data))
	for tokenID, txs := range resp.Data {
		respSummary[tokenID] = len(txs)
	}
	c.log.Debug("SyncTransactionChainsFromPeer: Received response from peer",
		"peerDID", peerDID,
		"tokenCount", len(resp.Data),
		"txCountByToken", respSummary,
		"message", resp.Message,
	)

	for tokenID, txs := range resp.Data {
		c.log.Info("SyncTransactionChainsFromPeer: Applying chain for token",
			"tokenID", tokenID,
			"txCount", len(txs),
		)
		prevTxID := prevTxIDs[tokenID] // empty string if not in map — applyTokenChainFromSync handles this
		if isFullnode {
			// The peer returns the chain as it stands on the peer, which can
			// include transactions this fullnode has received but not yet
			// validated. Persisting one of those advances the local tip past
			// what that transaction itself expects, and it then fails its own
			// integrity check because of a sync run for a different
			// transaction. Trim to the prefix that predates anything in flight.
			txs = c.txnProcessor.GuardAgainstInflight(tokenID, txs)
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

// syncTokensFromFullnode fetches the given tokens' chains from an authoritative
// full node (selected from the canonical fullnodes list) and returns, for each
// token the full node knows, the ID of the transaction that BURNT it (the last
// role=burn entry), or "" if the full node's chain shows it as not burnt.
//
// This exists so consensus validation can verify a burnt/committed parent
// token's TRUE latest state instead of trusting the transaction initiator's
// view — a rolled-back initiator can serve a stale (pre-burn) parent chain and
// drive a replayed split through consensus.
//
// It deliberately does NOT persist the fetched chain into the local wallet.
// The only consumer (ValidateSplitParentsAgainstFullnode) needs a single fact
// per ancestor — "which tx burnt it" — which the SyncedTxn response already
// carries (id + role per entry). Persisting via applyTokenChainFromSync would
// (a) require the transaction SIGNATURE, which the fullnode sync-info endpoint
// (SyncedTxn) does not transmit — the insert into `transactions` (signature NOT
// NULL) would fail — and (b) pollute this node's wallet with fullnode chains for
// tokens it does not own. Reading the burn tx straight from the response avoids
// both. (See the commented-out apply-based variant below for the alternative
// that would additionally need the SyncedTxn signature plumbing.)
//
// The fullnode sync endpoint (APISyncTransactionInfoFromFullnode) is any-peer
// callable and requires no ownership proof, so unlike wallet recovery this does
// not fetch/sign a nonce. It paginates until all pages are consumed.
//
// Returned map keys are only the tokens the fullnode actually returned data
// for; a token absent from the map means the fullnode had no chain for it.
func (c *Core) syncTokensFromFullnode(tokenIDs []string) (map[string]string, error) {
	if len(tokenIDs) == 0 {
		return map[string]string{}, nil
	}

	peerID, err := c.selectActiveFullnodePeer(c.Ctx)
	if err != nil {
		return nil, fmt.Errorf("syncTokensFromFullnode: select fullnode peer: %w", err)
	}

	peer, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		return nil, fmt.Errorf("syncTokensFromFullnode: connect to fullnode %s: %w", peerID, err)
	}
	defer peer.Close()

	c.log.Info("syncTokensFromFullnode: syncing parent tokens from fullnode",
		"fullnode_peer", peerID, "tokenCount", len(tokenIDs))

	burnRole := int16(models.GetTokenRoleID(constants.TokenRole_Burn))
	// burnTxByToken[tokenID] = the ID of the last role=burn entry the fullnode
	// served for that token; "" recorded when the fullnode returned the chain
	// but it has no burn entry (token still free on the authoritative record).
	burnTxByToken := make(map[string]string, len(tokenIDs))
	tokensInResponse := make(map[string]struct{}, len(tokenIDs))

	pageNumber := 1
	for {
		syncReq := types.SyncTransactionInfoFromFullnodeRequest{
			TokenIDs:   tokenIDs,
			PageNumber: pageNumber,
		}
		// The fullnode wraps the result in a BasicResponse envelope
		// ({status, message, result}), and SendJSONRequest decodes the raw body
		// into the target WITHOUT unwrapping. Decoding directly into a bare
		// SyncTransactionInfoFromFullnodeResult therefore silently yields a
		// zero-value (Data=nil, TotalItems=0) — the payload lives under "result".
		// Decode into the envelope and read the typed Result.
		var syncEnvelope struct {
			Status  bool                                        `json:"status"`
			Message string                                      `json:"message"`
			Result  types.SyncTransactionInfoFromFullnodeResult `json:"result"`
		}
		if err := peer.SendJSONRequest("POST", setup.APISyncTransactionInfoFromFullnode, nil, &syncReq, &syncEnvelope, false, 30*time.Second); err != nil {
			return nil, fmt.Errorf("syncTokensFromFullnode: request page %d from %s: %w", pageNumber, peerID, err)
		}
		syncResp := syncEnvelope.Result

		c.log.Debug("syncTokensFromFullnode: page received",
			"fullnode_peer", peerID, "page", pageNumber, "totalPages", syncResp.TotalPages,
			"tokensInPage", len(syncResp.Data), "totalItems", syncResp.TotalItems,
			"divergentTokens", syncResp.DivergentTokens)

		for tokenID, syncedTxns := range syncResp.Data {
			tokensInResponse[tokenID] = struct{}{}
			// Entries are position-ordered; the last role=burn entry is the tx
			// that consumed this token on the authoritative record. We read it
			// straight from the response — no local persistence, no signature.
			for _, st := range syncedTxns {
				if st.Role == burnRole {
					burnTxByToken[tokenID] = st.ID
				}
			}
			if _, seen := burnTxByToken[tokenID]; !seen {
				// Chain served but no burn entry → authoritatively not burnt.
				burnTxByToken[tokenID] = ""
			}

			// --- OPTION A (apply-based) — INTENTIONALLY DISABLED --------------
			// The alternative below persists the authoritative chain locally via
			// applyTokenChainFromSync, so getParentBurnTxID could re-read it from
			// the tokenchain table. It is disabled because it requires the
			// transaction SIGNATURE (transactions.signature is NOT NULL), which
			// the fullnode SyncedTxn payload does not currently carry — enabling
			// it needs the SyncedTxn signature plumbing on the fullnode
			// sync-serve path. It also writes fullnode chains into this node's
			// wallet for tokens it does not own. Kept for reference only.
			//
			// remoteTxs := make([]types.TransactionWithRole, 0, len(syncedTxns))
			// for _, st := range syncedTxns {
			// 	remoteTxs = append(remoteTxs, types.TransactionWithRole{
			// 		// Signature: st.Signature, // requires SyncedTxn.Signature (see Option A note)
			// 		Tx:   models.Transactions{ID: st.ID, Info: st.Info},
			// 		Role: st.Role,
			// 	})
			// }
			// if len(remoteTxs) > 0 {
			// 	if applyErr := c.applyTokenChainFromSync(tokenID, remoteTxs, "", false); applyErr != nil {
			// 		c.log.Warn("syncTokensFromFullnode: apply failed (non-fatal)", "tokenID", tokenID, "err", applyErr)
			// 	}
			// }
			// ------------------------------------------------------------------
		}

		if syncResp.TotalPages <= 0 || pageNumber >= syncResp.TotalPages {
			break
		}
		pageNumber++
	}

	// Surface tokens the fullnode returned NOTHING for — the authoritative
	// source has no chain to serve for them. This is the signal that made the
	// replayed-split check accept: an empty fullnode response is indistinguishable
	// from "ancestor is free" unless we log it explicitly.
	missing := make([]string, 0)
	for _, t := range tokenIDs {
		if _, ok := tokensInResponse[t]; !ok {
			missing = append(missing, t)
		}
	}
	c.log.Info("syncTokensFromFullnode: sync completed",
		"fullnode_peer", peerID, "tokenCount", len(tokenIDs),
		"tokensReturnedByFullnode", len(tokensInResponse),
		"tokensFullnodeHadNothingFor", missing, "burnTxByToken", burnTxByToken)
	return burnTxByToken, nil
}

// getTransactionInfoByID fetches a stored transaction by ID and returns its
// unmarshalled TransactionInfo. Used by the replayed-split check to read the
// burnt parents carried in a split genesis transaction's CommittedTokens.
// Returns an error if the transaction is not present locally or cannot be
// decoded; callers treat a miss as "not a resolvable split genesis".
func (c *Core) getTransactionInfoByID(txID string) (*models.TransactionInfo, error) {
	if txID == "" {
		return nil, fmt.Errorf("getTransactionInfoByID: empty transaction id")
	}
	txn, err := c.w.GetTransactionByID(txID, c.fullNode)
	if err != nil {
		return nil, err
	}
	var info models.TransactionInfo
	if err := json.Unmarshal(txn.Info, &info); err != nil {
		return nil, fmt.Errorf("getTransactionInfoByID: unmarshal info for %s: %w", txID, err)
	}
	return &info, nil
}

// getParentBurnTxID reads a parent token's chain and returns the ID of the
// transaction that burnt it (the last row with role=burn), if any. Used by the
// replayed-split check to confirm a parent was burnt by the split genesis that
// claims it, and not by an earlier, different transaction. A parent that is not
// burnt in the local (or freshly-synced) chain returns found=false.
func (c *Core) getParentBurnTxID(parentID string) (string, bool, error) {
	if parentID == "" {
		return "", false, nil
	}
	chain, err := c.w.GetTokenChainByTokenID(parentID, c.fullNode)
	if err != nil {
		// No chain locally is not an error for this check — the caller falls
		// back to an authoritative sync.
		c.log.Debug("getParentBurnTxID: no local chain for token (unconfirmed)", "tokenID", parentID, "err", err)
		return "", false, nil
	}
	burnRole := int16(models.GetTokenRoleID(constants.TokenRole_Burn))
	burnTxID := ""
	for _, row := range chain {
		if row.Role == burnRole {
			burnTxID = row.TransactionID
		}
	}
	c.log.Debug("getParentBurnTxID: read token chain",
		"tokenID", parentID, "chainLen", len(chain), "burnTxID", burnTxID, "burnt", burnTxID != "")
	return burnTxID, burnTxID != "", nil
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
		// A consumed parent must not default to Free, or it can be re-selected by
		// LockTokensForSplit and re-split into a duplicate genesis (double-mint).
		// Mirror the originating node: Commit -> Committed, Burn -> Burnt.
		case int16(models.GetTokenRoleID(constants.TokenRole_Commit)):
			tokenStatus = int16(constants.TokenStatus_Committed)
		case int16(models.GetTokenRoleID(constants.TokenRole_Burn)):
			tokenStatus = int16(constants.TokenStatus_Burnt)
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
	// A consumed parent must stay consumed after sync. This update runs even
	// for pre-existing tokens, so without these cases a re-synced burnt parent
	// would be overwritten back to Free and become re-splittable (double-mint).
	// Mirror the originating node: Commit -> Committed, Burn -> Burnt.
	case int16(models.GetTokenRoleID(constants.TokenRole_Commit)):
		lastStatus = int16(constants.TokenStatus_Committed)
	case int16(models.GetTokenRoleID(constants.TokenRole_Burn)):
		lastStatus = int16(constants.TokenStatus_Burnt)
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
