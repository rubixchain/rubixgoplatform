package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APISetupQuorum(req *ensweb.Request) *ensweb.Result {
	var qs model.QuorumSetup
	err := s.ParseJSON(req, &qs)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse the input", nil)
	}
	err = s.c.SetupQuorum(qs.DID, qs.Password)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to setup quorum, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Quorum setup done successfully", nil)
}

func (s *Server) APIUnpledgeTokens(req *ensweb.Request) *ensweb.Result {
	err := s.c.Up.RunUnpledge()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to unpledge, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Unpledged successfully", nil)
}

//func (s *Server) APIUnpinQuorumTokens(req *ensweb.Request) *ensweb.Result {
//	err := s.c.SetupQuorum(qs.DID, qs.Password)
//	if err != nil {
//		return s.BasicResponse(req, false, "Failed to setup quorum, "+err.Error(), nil)
//	}
//	return s.BasicResponse(req, true, "Quorum setup done successfully", nil)
//}
