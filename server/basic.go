package server

import (
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/dgrijalva/jwt-go"
)

// BasicResponse will send basic mode response
func (s *Server) BasicResponse(req *ensweb.Request, status bool, msg string, result interface{}) *ensweb.Result {
	resp := Response{
		Status:  status,
		Message: msg,
		Result:  result,
	}
	return s.RenderJSON(req, &resp, http.StatusOK)
}

// APILogin will setup the core
func (s *Server) APILogin(req *ensweb.Request) *ensweb.Result {
	s.log.Info("Received auth request")
	if !s.cfg.EnableAuth {
		return s.BasicResponse(req, false, "Authentication method not enabled", nil)
	}
	var lr LoginRequest
	err := s.ParseJSON(req, &lr)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	u, err := s.GetUser(req.TenantID, lr.UserName)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get user", nil)
	}
	isAdmin := false
	roles := make([]string, 0)
	for _, r := range u.Roles {
		if r.NormalizedName == "ADMIN" {
			isAdmin = true
		}
		roles = append(roles, r.NormalizedName)
	}

	expiresAt := time.Now().Add(time.Minute * 60).Unix()

	t := Token{
		u.ID.String(),
		isAdmin,
		roles,
		jwt.StandardClaims{
			ExpiresAt: expiresAt,
		},
	}
	token := s.GenerateJWTToken(t)

	switch s.cfg.AuthMethod {
	case SessionAuthMethod:
		err = s.SetSessionCookies(req, s.cfg.SessionName, s.cfg.SessionKey, token)
		if err != nil {
			s.log.Error("Failed to store token", "err", err)
			return s.BasicResponse(req, false, "Failed to store token", nil)
		}
		return s.BasicResponse(req, true, "User logged in successfully!", nil)
	default:
		return s.BasicResponse(req, false, "Authentication method is not implemented", nil)
	}
}

// APIStart will setup the core
func (s *Server) APIStart(req *ensweb.Request) *ensweb.Result {
	status, msg := s.c.Start()
	return s.BasicResponse(req, status, msg, nil)
}

// APIStart will setup the core
func (s *Server) APIShutdown(req *ensweb.Request) *ensweb.Result {
	go s.shutDown()
	return s.BasicResponse(req, true, "Shutting down...", nil)
}

func (s *Server) shutDown() {
	s.log.Info("Shutting down...")
	time.Sleep(2 * time.Second)
	s.sc <- true
}

// APIPing will ping to given peer
func (s *Server) APIPing(req *ensweb.Request) *ensweb.Result {
	peerdID := s.GetQuerry(req, "peerID")
	str, err := s.c.PingPeer(peerdID)
	if err != nil {
		s.log.Error("ping failed", "err", err)
		return s.BasicResponse(req, false, str, nil)
	}
	return s.BasicResponse(req, true, str, nil)
}
