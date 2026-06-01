package server

import (
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APISyncTransactionInfoFromFullnode godoc
// @Summary      Sync transaction info for tokens from fullnode tables
// @Description  Returns ordered transaction info for the requested token IDs from the complete fullnode history
// @Tags         tx
// @ID           syncTransactionInfoFromFullnode
// @Accept       json
// @Produce      json
// @Param        input body types.SyncTransactionInfoFromFullnodeRequest true "sync request"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/fullnode/sync-token-chain [post]
func (s *Server) APISyncTransactionInfoFromFullnode(req *ensweb.Request) *ensweb.Result {
	var syncReq types.SyncTransactionInfoFromFullnodeRequest
	err := s.ParseJSON(req, &syncReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if len(syncReq.TokenIDs) == 0 {
		return s.BasicResponse(req, true, "no token_ids provided", nil)
	}
	// Guard: cap batch size to prevent resource abuse on this public endpoint
	if len(syncReq.TokenIDs) > 50 {
		return s.BasicResponse(req, false, "max 50 token IDs per request", nil)
	}
	data, err := s.c.GetTransactionInfoFromFullnode(syncReq.TokenIDs)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.BasicResponse(req, true, "ok", data)
}
