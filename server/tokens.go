package server

import (
	"fmt"
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIGetAllTokens(req *ensweb.Request) *ensweb.Result {
	tokenType := s.GetQuerry(req, "type")
	did := s.GetQuerry(req, "did")
	tr, err := s.c.GetAllTokens(did, tokenType)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get tokens", nil)
	}
	return s.RenderJSON(req, tr, http.StatusOK)
}

func (s *Server) APIGenerateTestToken(req *ensweb.Request) *ensweb.Result {
	var tr model.RBTGenerateRequest
	err := s.ParseJSON(req, &tr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if !s.validateDIDAccess(req, tr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.GenerateTestTokens(req.ID, tr.NumberOfTokens, tr.DID)
	return s.didResponse(req, req.ID)
}

type RBTTransferRequestSwaggoInput struct {
	Receiver   string  `json:"receiver"`
	Sender     string  `json:"sender"`
	TokenCount float64 `json:"tokenCOunt"`
	Comment    string  `json:"comment"`
	Type       int     `json:"type"`
}

// ShowAccount godoc

// @Summary     Initiate RBT Transfer
// @Description This API will initiate RBT transfer to the specified dID
// @Tags        Account
// @ID 			initiate-rbt-transfer
// @Accept      json
// @Produce     json
// @Param 		input body RBTTransferRequestSwaggoInput true "Intitate RBT transfer"
// @Success 200 {object} model.BasicResponse
// @Router /api/initiate-rbt-transfer [post]
func (s *Server) APIInitiateRBTTransfer(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTTransferRequest
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(rbtReq.Sender)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateRBTTransfer(req.ID, &rbtReq)
	return s.didResponse(req, req.ID)
}

// function for Pinning RBT as service
func (s *Server) APIInitiatePinRBT(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTPinRequest
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(rbtReq.Sender)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiatePinRBT(req.ID, &rbtReq)
	return s.didResponse(req, req.ID)
}

// ShowAccount godoc
// @Summary      Check account balance
// @Description  For a mentioned DID, check the account balance
// @Tags         Account
// @Accept       json
// @Produce      json
// @Param        did      	   query      string  true  "User DID"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-account-info [get]
func (s *Server) APIGetAccountInfo(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	info, err := s.c.GetAccountInfo(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	ac := model.GetAccountInfo{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got account info successfully",
		},
		AccountInfo: make([]model.DIDAccountInfo, 0),
	}
	ac.AccountInfo = append(ac.AccountInfo, info)

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
// @Tags        Account
// @ID 			signature-response
// @Accept      json
// @Produce     json
// @Param 		input body SignatureResponseSwaggoInput true "Send input for requested signature"
// @Success 	200		{object}	model.BasicResponse
// @Router /api/signature-response [post]
func (s *Server) APISignatureResponse(req *ensweb.Request) *ensweb.Result {
	var resp did.SignRespData
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

// APIGetPledgedTokenDetails godoc
// @Summary     Get details about the pledged tokens
// @Description This API allows the user to get details about the tokens the quorums have pledged i.e. which token is pledged for which token state
// @Tags        Account
// @Produce     json
// @Success     200 {object} model.TokenStateResponse
// @Router      /api/get-pledgedtoken-details [get]
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

// APICheckPinnedState godoc
// @Summary     Check for exhausted token state hash
// @Description This API is used to check if the token state for which the token is pledged is exhausted or not.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       tokenstatehash	query	string	true	"Token State Hash"
// @Success 	200		{object}	model.BasicResponse
// @Router /api/check-pinned-state [delete]
func (s *Server) APICheckPinnedState(req *ensweb.Request) *ensweb.Result {
	tokenstatehash := s.GetQuerry(req, "tokenstatehash")

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
