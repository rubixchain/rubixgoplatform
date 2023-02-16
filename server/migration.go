package server

import (
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
	go s.handleWebRequest(req.ID)
	err = s.c.MigrateNode(req.ID, &m, didDir)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	br := model.BasicResponse{
		Status:  true,
		Message: "Node migrated successfully",
	}
	dc := s.c.GetWebReq(req.ID)
	dc.OutChan <- br
	time.Sleep(time.Millisecond * 10)
	s.c.RemoveWebReq(req.ID)
	return nil
}
