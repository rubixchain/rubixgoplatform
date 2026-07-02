package server

import (
	"strings"

	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary Recover wallet state from a published fullnode
// @Description Rebuilds the local wallet's token state for a DID by pulling
// owned tokens (status Free or Pledged) from an Active fullnode advertised at
// https://raw.githubusercontent.com/rubixchain/assets/refs/heads/main/fullnodes.json.
// Intended for nodes that lost their local DB and only retain their DID.
// Recovery proves DID ownership via a signed challenge, so this runs
// asynchronously through the signature request/response channel: the node
// returns "Signature needed", the caller signs the hash and POSTs it to
// /rubix/v1/signature, then the final recovery summary is returned.
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

	// Recovery signs an ownership challenge, so it runs asynchronously through
	// the OutChan/InChan signature mechanism (like register/transfer). The node
	// emits "Signature needed", the caller answers via /rubix/v1/signature, and
	// the final recovery summary is delivered on the same channel.
	s.c.AddWebReq(req)
	go s.c.RecoverWalletFromFullnodeAsync(req.ID, body.DID)
	return s.didResponse(req, req.ID)
}
