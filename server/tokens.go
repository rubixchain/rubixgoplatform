package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/dgrijalva/jwt-go"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
)

type Token struct {
	UserID  string   `json:"UserID"`
	IsAdmin bool     `json:"IsAdmin"`
	Roles   []string `json:"Roles"`
	jwt.StandardClaims
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

func (s *Server) APILockEpochTokens(req *ensweb.Request) *ensweb.Result {
	var tr model.RBTGenerateRequest
	err := s.ParseJSON(req, &tr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if !s.validateDIDAccess(req, tr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.W.LockAllTokens(tr.DID)
	return s.didResponse(req, req.ID)
}

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

func (s *Server) APIInitiateRBTSelfTransfer(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTSelfTransferRequest
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
	go s.c.InitiateRBTSelfTransfer(req.ID, &rbtReq)
	return s.didResponse(req, req.ID)
}

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
