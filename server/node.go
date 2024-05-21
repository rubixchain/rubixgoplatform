package server

import (
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIPeerID(req *ensweb.Request) *ensweb.Result {
	return s.BasicResponse(req, true, s.c.GetPeerID(), nil)
}
