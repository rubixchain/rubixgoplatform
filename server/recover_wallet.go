package server

import (
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/recovery"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary Recover wallet state from a published fullnode
// @Description Rebuilds a DID's local wallet by pulling its tokens and their
// transactions from an Active fullnode listed in the published fullnodes.json.
// A plain request with only a DID does a full recovery. Optional fields refine
// it: mode (full, delta, dryrun), token_types and token_ids filters, self_test,
// and all_dids to recover every local DID. Ownership is proven by a signed
// challenge, so the call runs asynchronously: the node returns "Signature
// needed", the caller signs the hash and POSTs it to /rubix/v1/signature, and
// the summary is returned on the same channel.
// @Tags Recovery
// @Accept json
// @Produce json
// @Param input body types.RecoverWalletAdvancedRequest true "Recovery options (did required unless all_dids is set)"
// @Success 200 {object} models.BasicResponse
// @Router /rubix/v1/sync [post]
func (s *Server) APIRecoverWalletFromFullnode(req *ensweb.Request) *ensweb.Result {
	var body types.RecoverWalletAdvancedRequest
	if err := s.ParseJSON(req, &body); err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	opts := recovery.RecoverOptions{
		Mode:       strings.TrimSpace(body.Mode),
		TokenTypes: body.TokenTypes,
		TokenIDs:   body.TokenIDs,
		SelfTest:   body.SelfTest,
	}

	// Whole-node recovery loops every local DID, so it carries no single DID.
	didStr := strings.TrimSpace(body.DID)
	if !body.AllDIDs {
		isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(didStr)
		if !strings.HasPrefix(didStr, "bafybmi") || len(didStr) != 59 || !isAlphanumeric {
			s.log.Error("Invalid DID", "did", didStr)
			return s.BasicResponse(req, false, "Invalid DID", nil)
		}
	}

	// Recovery signs an ownership challenge, so it runs through the OutChan/InChan
	// signature flow (like register/transfer) and the summary comes back on the
	// same channel.
	s.c.AddWebReq(req)
	go s.c.RecoverWalletFromFullnodeAsync(req.ID, didStr, opts, body.AllDIDs)
	return s.didResponse(req, req.ID)
}
