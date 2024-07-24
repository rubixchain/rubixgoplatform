package server

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APICreateFT(req *ensweb.Request) *ensweb.Result {
	var createFTReq model.CreateFTReq
	err := s.ParseJSON(req, &createFTReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if !s.validateDIDAccess(req, createFTReq.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.CreateFTs(req.ID, createFTReq.DID, createFTReq.FTCount, createFTReq.FTName, createFTReq.TokenCount)
	return s.didResponse(req, req.ID)
}
