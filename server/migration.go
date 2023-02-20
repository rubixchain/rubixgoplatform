package server

import (
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
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
	dc := s.c.GetWebReq(req.ID)
	go s.c.MigrateNode(req.ID, &m, didDir)

	ch := <-dc.OutChan
	time.Sleep(time.Millisecond * 10)
	br := ch.(model.BasicResponse)
	if !br.Status || br.Result == nil {
		s.c.RemoveWebReq(req.ID)
	}
	return s.RenderJSON(req, &br, http.StatusOK)
}
