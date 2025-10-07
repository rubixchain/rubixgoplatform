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

// @Summary     Unpledge POW Based pledge Tokens
// @Description This API will unpledge all PoW based pledge tokens and drop the unpledgequeue table
// @Tags        Account
// @ID 			unpledge-pow-unpledge-tokens
// @Accept      json
// @Produce     json
// @Success 200 {object} model.BasicResponse
// @Router 		/api/unpledge-pow-unpledge-tokens [post]
func (s *Server) UnpledgePoWBasedPledgedTokens(req *ensweb.Request) *ensweb.Result {
	var resp model.BasicResponse

	err := s.c.ForceUnpledgePOWBasedPledgedTokens()
	if err != nil {
		resp.Status = false
		resp.Message = err.Error()

		return s.BasicResponse(req, false, err.Error(), nil)
	}

	resp.Status = true
	resp.Message = "Unpledging of all PoW based pledged tokens is successful"
	return s.RenderJSON(req, resp, http.StatusOK)
}
