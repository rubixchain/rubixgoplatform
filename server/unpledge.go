package server

import (
	"fmt"
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary     Run Unpledge
// @Description This API will unpledge all Pledged RBT tokens
// @Tags        Account
// @ID 			run-unpledge
// @Accept      json
// @Produce     json
// @Success 200 {object} model.BasicResponse
// @Router /api/run-unpledge [post]
func (s *Server) RunUnpledgeHandle(req *ensweb.Request) *ensweb.Result {
	var resp model.BasicResponse

	var msg string
	var err error
	
	// Check if optimized unpledging is enabled
	if s.c.GetConfig() != nil && s.c.GetConfig().CfgData.EnableOptimizedUnpledge {
		msg, err = s.c.InitiateOptimizedUnpledgeProcess()
	} else {
		msg, err = s.c.InititateUnpledgeProcess()
	}
	
	if err != nil {
		errMsg := fmt.Sprintf("%v: %v", setup.APIRunUnpledge, err.Error())
		resp.Status = false
		resp.Message = errMsg

		return s.BasicResponse(req, false, errMsg, nil)
	}

	resp.Status = true
	resp.Message = msg
	return s.RenderJSON(req, resp, http.StatusOK)
}

