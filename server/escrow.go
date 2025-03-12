package server

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ShowAccount godoc
// @Summary      Initiate transfer from contract to DID
// @Description  This API endpoint will  Initiate transfer from contract to DID.
// @Tags         FT
// @Accept       json
// @Produce      json
// @Param        input body model.TransferToDidReq true "Transfer from contract to did"
// @Success      200  {object}  model.BasicResponse
// @Router      /rubix-core/v1/smart-contract/contract-to-did [post]
func (s *Server) APITransferToDid(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.TransferToDidReq
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(rbtReq.Did)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.TransferToDidRequest(req.ID, rbtReq)
	return s.didResponse(req, req.ID)
}
