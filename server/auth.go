package server

import "github.com/EnsurityTechnologies/ensweb"

// validateAccess : validate the access based on the client token,
// api key access will have rot directory access
func (s *Server) validateAccess(req *ensweb.Request) (string, bool) {
	if s.cfg.EnableAuth {
		if req.ClientToken.Verified {
			token := req.ClientToken.Model.(*Token)
			return token.UserID, true
		} else if req.ClientToken.APIKeyVerified {
			return DIDRootDir, true
		} else {
			return "", false
		}
	} else {
		return DIDRootDir, true
	}
}
