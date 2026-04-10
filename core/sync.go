package core

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// syncTxChainRequest is the request body for the batch token chain sync API.
type syncTxChainRequest struct {
	DID      string   `json:"did"`
	TokenIDs []string `json:"token_ids"`
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

	result := make(map[string][]models.Transactions)
	for _, tokenID := range req.TokenIDs {
		txs, err := c.w.GetTransactionsByTokenID(tokenID)
		if err != nil {
			c.log.Warn("SyncTransactionChain: failed to fetch chain", "tokenID", tokenID, "err", err)
			continue
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
// from the peer identified by peerDID, and inserts them into the local DB as-is.
// Duplicate transactions are silently skipped.
func (c *Core) SyncTransactionChainsFromPeer(peerDID string, tokenIDs []string) error {
	if len(tokenIDs) == 0 {
		return nil
	}

	req := syncTxChainRequest{DID: peerDID, TokenIDs: tokenIDs}

	p, err := c.getPeer(peerDID)
	if err != nil {
		return fmt.Errorf("SyncTransactionChainsFromPeer: getPeer failed: %w", err)
	}
	defer p.Close()

	var resp syncTxChainResponse
	if err = p.SendJSONRequest("POST", APISyncTransactionChain, nil, &req, &resp, false); err != nil {
		return fmt.Errorf("SyncTransactionChainsFromPeer: request failed: %w", err)
	}

	if !resp.Status {
		return fmt.Errorf("SyncTransactionChainsFromPeer: peer returned error: %s", resp.Message)
	}

	for tokenID, txs := range resp.Data {
		for _, t := range txs {
			tx := t
			if err := c.w.CreateTransaction(&tx); err != nil {
				if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
					// Already synced — skip silently
					continue
				}
				c.log.Warn("SyncTransactionChainsFromPeer: insert failed", "tokenID", tokenID, "txID", tx.ID, "err", err)
			}
		}
	}

	return nil
}
