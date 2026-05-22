package server

import (
	"fmt"
	"net/http"
	"regexp"
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

func (s *Server) APIDumpFTTokenChainBlock(req *ensweb.Request) *ensweb.Result {
	var dr model.TCDumpRequest
	err := s.ParseJSON(req, &dr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	drep := s.c.DumpFTTokenChain(&dr)
	return s.RenderJSON(req, drep, http.StatusOK)
}

// SmartContract godoc
// @Summary      Get FT Token Chain Data
// @Description  This API returns FT token chain data for a given FT token ID.
// @Tags         FT
// @Accept       json
// @Produce      json
// @Param        tokenID	query	string	true	"FT Token ID"
// @Success      200  {object}  model.GetTokenChainResponce "Successful response with token chain data"
// @Router       /api/get-ft-token-chain [get]
func (s *Server) APIGetFTTokenchain(req *ensweb.Request) *ensweb.Result {
	TokenID := s.GetQuery(req, "tokenID")
	if TokenID == "" {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(TokenID)
	if len(TokenID) != 46 || !strings.HasPrefix(TokenID, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid FT token")
		return s.BasicResponse(req, false, "Invalid FT token ID", nil)
	}
	getResp := s.c.GetFTTokenchain(TokenID)
	return s.RenderJSON(req, getResp, http.StatusOK)
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

func (s *Server) APIUpdateTokenStatus(req *ensweb.Request) *ensweb.Result {
	var updateReq model.UpdateTokenStatusReq
	err := s.ParseJSON(req, &updateReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	err = s.c.UpdateTokenStatus(&updateReq)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to update token status", err)
	}
	return s.RenderJSON(req, model.BasicResponse{Message: "Token status updated successfully", Status: true}, http.StatusOK)
}

func (s *Server) APIGetTokenStatus(req *ensweb.Request) *ensweb.Result {
	var getTokenStatusReq model.GetTokenStatusReq
	err := s.ParseJSON(req, &getTokenStatusReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	var response model.TokenStatusResponse
	response, err = s.c.GetTokenStatus(&getTokenStatusReq)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get token status", nil)
	}
	return s.RenderJSON(req, response, http.StatusOK)
}
