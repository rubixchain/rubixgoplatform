package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIDumpTokenChainBlock(req *ensweb.Request) *ensweb.Result {
	var dr model.TCDumpRequest
	err := s.ParseJSON(req, &dr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}

	elems := strings.Split(dr.Token, "_")
	if len(elems) < 2 {
		s.log.Error(fmt.Sprintf("Invalid token: %v", dr.Token))
		return s.BasicResponse(req, false, "Invalid token", nil)
	}
	drep := s.c.DumpTokenChain(&dr)
	return s.RenderJSON(req, drep, http.StatusOK)
}

type RegisterCallBackURLSwaggoInput struct {
	Token       string `json:"SmartContractToken"`
	CallBackURL string `json:"CallBackURL"`
}

// SmartContract godoc
// @Summary      Get Smart Contract Token Chain Data
// @Description  This API will register call back url for when updated come for smart contract token
// @Tags         Smart Contract
// @ID 			 register-callback-url
// @Accept       json
// @Produce      json
// @Param		 input body RegisterCallBackURLSwaggoInput true "Register call back URL"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/register-callback-url [post]
func (s *Server) APIRegisterCallbackURL(req *ensweb.Request) *ensweb.Result {
	var registerReq model.RegisterCallBackUrlReq
	err := s.ParseJSON(req, &registerReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	response := s.c.RegisterCallBackURL(&registerReq)
	return s.RenderJSON(req, response, http.StatusOK)
}
