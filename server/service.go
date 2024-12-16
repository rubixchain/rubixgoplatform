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

// APIUpdateConfigPort will update the config port
// @Summary Update config port
// @Description Update config port
// @Tags config
// @Accept  json
// @Produce  json
// @Success 200 {object} ensweb.Result	"Config updated successfully"
// @Failure 400 {object} ensweb.Result	"Failed to update config"
// @Router /api/update-config [post]
func (s *Server) APIUpdateConfigPort(req *ensweb.Request) *ensweb.Result {
	var m config.UpdateConfigPort
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid request, failed to update config", nil)
	}
	err = s.c.UpdateConfigPort(&m)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to update config", nil)
	}
	return s.BasicResponse(req, true, "Config updated successfully", nil)

}
