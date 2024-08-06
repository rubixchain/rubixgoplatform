package server

import (
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

type RBTSelfTransferRequestSwaggoInput struct {
	Sender     string  `json:"sender"`
	Type       int     `json:"type"`
}

// @Summary     Initiate Self Transfer
// @Description This API will initiate self RBT transfer for a specific DID
// @Tags        Account
// @ID 			initiate-self-transfer
// @Accept      json
// @Produce     json
// @Param 		input body RBTSelfTransferRequestSwaggoInput true "Intitate Self RBT transfer"
// @Success 200 {object} model.BasicResponse
// @Router /api/initiate-self-transfer [post]
func (s *Server) SelfTransferHandle(req *ensweb.Request) *ensweb.Result {
	var selfTransferReq model.RBTTransferRequest
	err := s.ParseJSON(req, &selfTransferReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}

	senderDID := selfTransferReq.Sender
	

	if !s.validateDIDAccess(req, senderDID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	
	// Make receiver to be same as sender for Self Transfer
	selfTransferReq.Receiver = selfTransferReq.Sender
	s.c.AddWebReq(req)

	go s.c.InitiateRBTTransfer(req.ID, &selfTransferReq)
	return s.didResponse(req, req.ID)
}