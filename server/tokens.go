package server

import (
	"encoding/json"
	"net/http"
	"time"

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
	go s.handleWebRequest(req.ID)
	err = s.c.GenerateTestTokens(req.ID, tr.NumberOfTokens, tr.DID)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	br := model.BasicResponse{
		Status:  true,
		Message: "Token generated successfully",
	}
	dc := s.c.GetWebReq(req.ID)
	dc.OutChan <- br
	time.Sleep(time.Millisecond * 10)
	s.c.RemoveWebReq(req.ID)
	return nil
}

func (s *Server) APIInitiateRBTTransfer(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTTransferRequest
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	s.c.AddWebReq(req)
	go s.handleWebRequest(req.ID)
	br := s.c.InitiateRBTTransfer(req.ID, &rbtReq)
	dc := s.c.GetWebReq(req.ID)
	dc.OutChan <- br
	time.Sleep(time.Millisecond * 10)
	s.c.RemoveWebReq(req.ID)
	return nil
}

func (s *Server) handleWebRequest(id string) {
	dc := s.c.GetWebReq(id)
	var ch interface{}
	select {
	case ch = <-dc.OutChan:
		w := dc.Req.GetHTTPWritter()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		enc := json.NewEncoder(w)
		enc.Encode(ch)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	case <-dc.Finish:
	case <-time.After(time.Minute * 10):
	}
}

func (s *Server) APIGetAccountInfo(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	info, err := s.c.GetAccountInfo(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.RenderJSON(req, info, http.StatusOK)
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
	dc.InChan <- resp
	s.log.Debug("Singature trasnfered", "resp", resp)
	if resp.Mode == did.BasicDIDMode {
		s.handleWebRequest(resp.ID)
		return nil
	}
	// ::TODO:: Need to updated for other mode
	return s.BasicResponse(req, false, "Signature processed", nil)
}
