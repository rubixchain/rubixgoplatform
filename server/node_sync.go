package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APINodeSync(req *ensweb.Request) *ensweb.Result {
	var restoreNodeSyncRequest model.NodeSyncRequest
	err := s.ParseJSON(req, &restoreNodeSyncRequest)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	resp := s.c.NodeSync(&restoreNodeSyncRequest)
	return s.RenderJSON(req, resp, http.StatusOK)
}
