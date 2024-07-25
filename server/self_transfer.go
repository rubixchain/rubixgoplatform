package server

import (
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// @Summary     Initiate Self Transfer
// @Description This API will initiate self RBT transfer for a specific DID
// @Tags        Account
// @ID 			initiate-self-transfer
// @Accept      json
// @Produce     json
// @Param 		input body RBTTransferRequestSwaggoInput true "Intitate RBT transfer"
// @Success 200 {object} model.BasicResponse
// @Router /api/initiate-self-transfer [post]
func (s *Server) SelfTransferHandle(req *ensweb.Request) *ensweb.Result {
	var selfTransferReq model.RBTTransferRequest
	err := s.ParseJSON(req, &selfTransferReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}

	senderDID := selfTransferReq.Sender
	receiverDID := selfTransferReq.Receiver

	if receiverDID != "" && senderDID != receiverDID {
		return s.BasicResponse(req, false, "Sender and Receiver must be same in case of self transfer", nil)
	}

	if !s.validateDIDAccess(req, senderDID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	
	s.c.AddWebReq(req)

	go s.c.InitiateRBTTransfer(req.ID, &selfTransferReq)
	return s.didResponse(req, req.ID)
}