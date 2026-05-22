package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

var jwtSecret = []byte("RubixBIPWallet")

// validateAccess : validate the access based on the client token,
// api key access will have rot directory access
func (s *Server) validateAccess(req *ensweb.Request) bool {
	if s.cfg.EnableAuth {
		if req.ClientToken.Verified {
			// token := req.ClientToken.Model.(*setup.BearerToken)
			return true
		} else if req.ClientToken.APIKeyVerified {
			return true
		} else {
			return false
		}
	} else {
		return true
	}
}

func (s *Server) AuthError(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "unauthorized access"}, http.StatusUnauthorized)
}
