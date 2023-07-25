package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
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
// @Param 		receiver 	body string true "The decentralized identifier of the receiver"
// @Param 		sender 		body string true "The decentralized identifier of the sender"
// @Param 		tokenCount 	body number true "The number of RBT tokens to transfer"
// @Param 		comment 	body string false "A comment for the transfer"
// @Param 		type 		body int true "The type of transfer (1 for direct transfer, 2 for group transfer)"
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

type inputData struct {
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
// @Param 		id			body	string	true 	"Req ID"
// @Param		mode		body	int		true	"Mode of the node"
// @Param		password	body	string	true	"password of the node"
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
