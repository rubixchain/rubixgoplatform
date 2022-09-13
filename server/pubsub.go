package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/config"
)

func (s *Server) APIEnableExplorer(req *ensweb.Request) *ensweb.Result {
	var m config.ExplorerConfig
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid request, failed to subscribe explorer", nil)
	}
	err = s.c.ConfigureExplorer(&m)
	if err != nil {
		return s.BasicResponse(req, false, "failed to enable explorer", nil)
	}
	return s.BasicResponse(req, true, "Explorer service enabled successfully", nil)
}
