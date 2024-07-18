package server

import "github.com/rubixchain/rubixgoplatform/wrapper/ensweb"

func (s *Server) APICreateFT(req *ensweb.Request) *ensweb.Result {
	return s.didResponse(req, req.ID)
}
