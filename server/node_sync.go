package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APINodeSync(req *ensweb.Request) *ensweb.Result {
	var nodeSyncRequest model.NodeSyncRequest
	err := s.ParseJSON(req, &nodeSyncRequest)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	resp := s.c.NodeSync(&nodeSyncRequest)
	return s.RenderJSON(req, resp, http.StatusOK)
}
