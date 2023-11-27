package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIMigrateNode(req *ensweb.Request) *ensweb.Result {
	var m core.MigrateRequest
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	didDir := DIDRootDir
	if s.cfg.EnableAuth {
		// always expect client tokne to present
		token := req.ClientToken.Model.(*setup.BearerToken)
		didDir = token.DID
	}
	s.c.AddWebReq(req)
	go s.c.MigrateNode(req.ID, &m, didDir)
	return s.didResponse(req, req.ID)
}

func (s *Server) APILockTokens(req *ensweb.Request) *ensweb.Result {
	var ts []string
	if !s.c.IsArbitaryMode() {
		return s.BasicResponse(req, false, "Only allowed in arbitary mode", nil)
	}
	err := s.ParseJSON(req, &ts)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	br := s.c.LockTokens(ts)
	return s.RenderJSON(req, br, http.StatusOK)
}
