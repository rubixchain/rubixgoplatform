package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/dgrijalva/jwt-go"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/model"
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
	s.c.AddWebReq(req)
	err = s.c.GenerateTestTokens(req.ID, tr.NumberOfTokens, tr.DID)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	sreq := s.c.RemoveWebReq(req.ID)
	return s.BasicResponse(sreq, true, "Token generated successfully", nil)
}

func (s *Server) APIInitiateRBTTransfer(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTTransferRequest
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	s.c.AddWebReq(req)
	br := s.c.InitiateRBTTransfer(req.ID, &rbtReq)
	sreq := s.c.RemoveWebReq(req.ID)
	return s.RenderJSON(sreq, br, http.StatusOK)
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
	if resp.Mode == did.BasicDIDMode {
		s.c.UpateWebReq(resp.ID, req)
	}
	s.log.Debug("Received Singature response", "resp", resp)
	dc.Chan <- resp
	s.log.Debug("Singature trasnfered", "resp", resp)
	if resp.Mode == did.BasicDIDMode {
		return &ensweb.Result{Status: http.StatusOK, Done: true}
	}
	// ::TODO:: Need to updated for other mode
	return s.BasicResponse(req, false, "Signature processed", nil)
}
