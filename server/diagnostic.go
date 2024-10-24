package server

import (
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
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(dr.Token)
	if len(dr.Token) != 46 || !strings.HasPrefix(dr.Token, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid token")
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
// @Success      200  {object}  model.GetFTTokenChainReply "Successful response with token chain data"
// @Router       /api/get-ft-token-chain [get]
func (s *Server) APIGetFTTokenchain(req *ensweb.Request) *ensweb.Result {
	TokenID := s.GetQuerry(req, "tokenID")
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

func (s *Server) APIDumpSmartContractTokenChainBlock(req *ensweb.Request) *ensweb.Result {
	var dr model.TCDumpRequest
	err := s.ParseJSON(req, &dr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(dr.Token)
	if len(dr.Token) != 46 || !strings.HasPrefix(dr.Token, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid smart contract token")
		return s.BasicResponse(req, false, "Invalid smart contract token", nil)
	}
	drep := s.c.DumpSmartContractTokenChain(&dr)
	return s.RenderJSON(req, drep, http.StatusOK)
}

type GetSmartContractTokenChainDataSwaggoInput struct {
	Token  string `json:"token"`
	Latest bool   `json:"latest"`
}

// SmartContract godoc
// @Summary      Get Smart Contract Token Chain Data
// @Description  This API will return smart contract token chain data
// @Tags         Smart Contract
// @ID 			 get-smart-contract-token-chain-data
// @Accept       json
// @Produce      json
// @Param		 input body GetSmartContractTokenChainDataSwaggoInput true "Returns Smart contract token chain Execution Data"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/get-smart-contract-token-chain-data [post]
func (s *Server) APIGetSmartContractTokenChainData(req *ensweb.Request) *ensweb.Result {
	var getReq model.SmartContractTokenChainDataReq
	err := s.ParseJSON(req, &getReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	sctdataReply := s.c.GetSmartContractTokenChainData(&getReq)
	return s.RenderJSON(req, sctdataReply, http.StatusOK)
}

type RegisterCallBackURLSwaggoInput struct {
	Token       string `json:"token"`
	CallBackURL string `json:"callbackurl"`
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

func (s *Server) APIRemoveTokenChainBlock(req *ensweb.Request) *ensweb.Result {
	var removeReq model.TCRemoveRequest
	err := s.ParseJSON(req, &removeReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	removeReply := s.c.RemoveTokenChainBlock(&removeReq)
	return s.RenderJSON(req, removeReply, http.StatusOK)
}

func (s *Server) APIReleaseAllLockedTokens(req *ensweb.Request) *ensweb.Result {
	var response model.BasicResponse
	response = s.c.ReleaseAllLockedTokens()
	return s.RenderJSON(req, response, http.StatusOK)
}
