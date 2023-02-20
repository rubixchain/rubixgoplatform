package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core"
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
		token := req.ClientToken.Model.(*Token)
		didDir = token.UserID
	}
	s.c.AddWebReq(req)
	go s.c.MigrateNode(req.ID, &m, didDir)
	return s.didResponse(req, req.ID)
}
