package server

import (
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APISetupService(req *ensweb.Request) *ensweb.Result {
	var m config.ServiceConfig
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid request, failed to subscribe service", nil)
	}
	err = s.c.ConfigureService(&m)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to enable service", nil)
	}
	return s.BasicResponse(req, true, "Service enabled successfully", nil)
}
