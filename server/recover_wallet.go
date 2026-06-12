package server

import (
	"strings"

	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary Recover wallet state from a published fullnode
// @Description Rebuilds the local wallet's token state for a DID by pulling
// owned tokens (status Free or Pledged) from an Active fullnode advertised at
// https://raw.githubusercontent.com/rubixchain/assets/refs/heads/main/fullnodes.json.
// Intended for nodes that lost their local DB and only retain their DID. The
// request is DID-signed end-to-end inside the core; the HTTP layer only needs
// the DID string.
// @Tags Recovery
// @Accept json
// @Produce json
// @Param did body string true "DID of the wallet to recover"
// @Success 200 {object} model.BasicResponse
// @Router /api/recover-wallet-from-fullnode [post]
func (s *Server) APIRecoverWalletFromFullnode(req *ensweb.Request) *ensweb.Result {
	body := struct {
		DID string `json:"did"`
	}{}
	if err := s.ParseJSON(req, &body); err != nil {
		// Fallback: ensweb sometimes routes form/query data through req.Data.
		if req.Data != nil {
			if v, ok := req.Data["did"].(string); ok {
				body.DID = v
			}
		}
	}
	body.DID = strings.TrimSpace(body.DID)
	if body.DID == "" {
		return s.BasicResponse(req, false, "did is required", nil)
	}

	result, err := s.c.RecoverWalletFromFullnode(s.c.Ctx, body.DID)
	if err != nil {
		s.log.Error("APIRecoverWalletFromFullnode: recovery failed",
			"did", body.DID, "err", err)
		return s.BasicResponse(req, false, "recovery failed: "+err.Error(), result)
	}

	s.log.Info("APIRecoverWalletFromFullnode: recovery completed",
		"did", body.DID,
		"fullnode_peer", result.FullnodePeerID,
		"tokens_seen", result.TokensSeen,
		"chain_entries_persisted", result.ChainEntriesPersisted,
		"tokens_failed", result.TokensFailed,
		"pages_fetched", result.PagesFetched,
		"divergent_tokens", len(result.DivergentTokens))
	return s.BasicResponse(req, true, "wallet recovery completed", result)
}
