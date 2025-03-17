package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// validateAccess : validate the access based on the client token,
// api key access will have rot directory access
func (s *Server) validateAccess(req *ensweb.Request) (string, bool) {
	if s.cfg.EnableAuth {
		if req.ClientToken.Verified {
			token := req.ClientToken.Model.(*setup.BearerToken)
			return token.DID, true
		} else if req.ClientToken.APIKeyVerified {
			return DIDRootDir, true
		} else {
			return "", false
		}
	} else {
		return DIDRootDir, true
	}
}

func (s *Server) AuthError(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "unauthorized access"}, http.StatusUnauthorized)
}

func (s *Server) DIDAuthHandle(hf ensweb.HandlerFunc, af ensweb.AuthFunc, ef ensweb.HandlerFunc, root bool) ensweb.HandlerFunc {
	return ensweb.HandlerFunc(func(req *ensweb.Request) *ensweb.Result {
		bt, ok := s.c.ValidateDIDToken(req.ClientToken.Token, setup.AccessTokenType, "")
		if !ok {
			if ef != nil {
				return ef(req)
			} else {
				return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "ivnalid token"}, http.StatusUnauthorized)
			}
		}
		if root && !bt.Root {
			return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "root access denied"}, http.StatusUnauthorized)
		}
		req.ClientToken.Model = bt
		req.ClientToken.Verified = true
		if af != nil {
			if !af(req) {
				if ef != nil {
					return ef(req)
				} else {
					return s.RenderJSONError(req, http.StatusForbidden, "Access denined", "Access denied")
				}
			}
		}
		return hf(req)
	})
}
