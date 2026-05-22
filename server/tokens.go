package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIGetAllTokens(req *ensweb.Request) *ensweb.Result {
	tokenType := s.GetQuery(req, "type")
	did := s.GetQuery(req, "did")
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	tr, err := s.c.GetAllTokens(did, tokenType)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get tokens", nil)
	}
	return s.RenderJSON(req, tr, http.StatusOK)
}

func (s *Server) APIGenerateLocalRBT(req *ensweb.Request) *ensweb.Result {
	var tr model.GenerateLocalRBTRequest
	err := s.ParseJSON(req, &tr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(tr.DID)
	if !strings.HasPrefix(tr.DID, "bafybmi") || len(tr.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	if tr.NumberOfTokens <= 0 {
		s.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
		return s.BasicResponse(req, false, "Invalid RBT amount, tokens generated should be a whole number and greater than 0", nil)
	}

	if !s.validateDIDAccess(req, tr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.GenerateLocalRBT(req.ID, tr.NumberOfTokens, tr.DID, tr.StartIndex)
	return s.didResponse(req, req.ID)
}

func (s *Server) APIGenerateMainnetRBT(req *ensweb.Request) *ensweb.Result {
	var tr model.GenerateLocalRBTRequest
	err := s.ParseJSON(req, &tr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(tr.DID)
	if !strings.HasPrefix(tr.DID, "bafybmi") || len(tr.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	if tr.NumberOfTokens <= 0 {
		s.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
		return s.BasicResponse(req, false, "Invalid RBT amount, tokens generated should be a whole number and greater than 0", nil)
	}
	if !s.validateDIDAccess(req, tr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.GenerateMainnetRBT(req.ID, tr.NumberOfTokens, tr.DID, tr.StartIndex)
	return s.didResponse(req, req.ID)
}

type RBTTransferRequestSwaggoInput struct {
	Receiver   string  `json:"receiver"`
	Sender     string  `json:"sender"`
	TokenCount float64 `json:"tokenCOunt"`
	Comment    string  `json:"comment"`
	Type       int     `json:"type"`
}

// GetRBTBalance godoc
// @Summary      Get RBT Balance
// @Description  Retrieves the RBT token balance for a given DID.
// @Tags         DID
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID (e.g. did:bafybmih3l2emb4s7wbsgakwv4voaqngdirpg5f3kqlheqqsgdg7jthuwaq)"
// @Success      200  {object}  model.BasicResponse
// @Failure      400  {object}  model.BasicResponse
// @Router       /rubix/v1/dids/{did}/balances/rbt [get]
func (s *Server) APIGetRbtByDid(req *ensweb.Request) *ensweb.Result {
	did := s.GetRouteVar(req, "did")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID:", did)
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	info, err := s.c.GetRbtByDid(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	ac := model.BasicResponse{
		Status:  true,
		Message: "Got account info successfully",
		Result:  info,
	}
	return s.RenderJSON(req, ac, http.StatusOK)
}

type SignatureResponseSwaggoInput struct {
	ID       string `json:"id"`
	Mode     int    `json:"mode"`
	Password string `json:"password"`
}

// ShowAccount godoc
// @Summary     Signature Response
// @Description This API is used to supply the password for the node along with the ID generated when Initiate RBT transfer is called.
// @Tags        Signature
// @ID 			signature-response
// @Accept      json
// @Produce     json
// @Param 		input body types.SignRespData true "Send input for requested signature"
// @Success      200      {object}  model.BasicResponse
// @Failure      400      {object}  model.BasicResponse
// @Router /rubix/v1/signature [post]
func (s *Server) APISignatureResponse(req *ensweb.Request) *ensweb.Result {
	var resp types.SignRespData
	err := s.ParseJSON(req, &resp)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	dc := s.c.GetWebReq(resp.ID)
	if dc == nil {
		return s.BasicResponse(req, false, "Invalid request ID", nil)
	}
	s.c.UpateWebReq(resp.ID, req)
	dc.InChan <- resp
	return s.didResponse(req, resp.ID)
}

func (s *Server) APIGetPledgedTokenDetails(req *ensweb.Request) *ensweb.Result {
	pledgedTokenInfo, err := s.c.GetPledgedInfo()
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	tokenstateresponse := model.TokenStateResponse{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got pledged tokens with token states info successfully",
		},
		PledgedTokenStateDetails: make([]model.PledgedTokenStateDetails, 0),
	}
	tokenstateresponse.PledgedTokenStateDetails = append(tokenstateresponse.PledgedTokenStateDetails, pledgedTokenInfo...)
	return s.RenderJSON(req, tokenstateresponse, http.StatusOK)
}

func (s *Server) APICheckPinnedState(req *ensweb.Request) *ensweb.Result {
	tokenstatehash := s.GetQuery(req, "tokenstatehash")

	provList, err := s.c.GetDHTddrs(tokenstatehash)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	var br model.BasicResponse
	if len(provList) == 0 {
		br.Status = false
		br.Message = fmt.Sprintf("No pins available on %s", tokenstatehash)
		return s.RenderJSON(req, br, http.StatusOK)
	} else {
		br.Status = true
		br.Result = provList
	}

	err = s.c.UpdatePledgedTokenInfo(tokenstatehash)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	br.Message = "Got Pins on " + tokenstatehash + ". Updated the pledging detail in table and removed from pledged token state table."
	return s.RenderJSON(req, br, http.StatusOK)
}

func (s *Server) APIGenerateFaucetTestToken(req *ensweb.Request) *ensweb.Result {
	var tr model.FaucetRBTGenerateRequest
	err := s.ParseJSON(req, &tr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(tr.DID)
	if !strings.HasPrefix(tr.DID, "bafybmi") || len(tr.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	if tr.TokenCount <= 0 {
		s.log.Error("Invalid level number, level should be greater than 0")
		return s.BasicResponse(req, false, "Invalid level number, level should be greater than 0", nil)
	}

	if !s.validateDIDAccess(req, tr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.GenerateFaucetTestTokens(req.ID, tr.TokenCount, tr.DID)
	return s.didResponse(req, req.ID)
}

func (s *Server) APIValidateToken(req *ensweb.Request) *ensweb.Result {
	token := s.GetQuery(req, "token")
	br, err := s.c.ValidateToken(token)
	if err != nil {
		s.log.Error("Failed to validate token ", err)
		return s.BasicResponse(req, false, "Failed to validate token : "+err.Error(), nil)
	}
	return s.RenderJSON(req, br, http.StatusOK)
}
