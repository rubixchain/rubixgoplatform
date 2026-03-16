package server

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIInitiateTransaction(req *ensweb.Request) *ensweb.Result {
	var transactionreq models.TransactionRequest
	err := s.ParseJSON(req, &transactionreq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateTransaction(req.ID, &transactionreq)
	return s.didResponse(req, req.ID)
}
