package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
)

func (s *Server) BasicResponse(req *ensweb.Request, status string, msg string) *ensweb.Result {
	resp := Repsonse{
		Data:   make(map[string]string),
		Status: status,
	}
	resp.Data[ReponseMsgHdr] = msg
	return s.RenderJSON(req, &resp, http.StatusOK)
}

// APIStart will starts the Alpha, Beta, Gamma Quorum receives and NFT & Token receivers
func (s *Server) APIStart(req *ensweb.Request) *ensweb.Result {
	status, msg := s.c.Start()
	return s.BasicResponse(req, status, msg)
}

// APIStart will starts the Alpha, Beta, Gamma Quorum receives and NFT & Token receivers
func (s *Server) APIPing(req *ensweb.Request) *ensweb.Result {
	peerdID := s.GetQuerry(req, "peerID")
	str, err := s.c.PingPeer(peerdID)
	if err != nil {
		s.log.Error("ping failed", "err", err)
		return s.BasicResponse(req, "false", str)
	}
	return s.BasicResponse(req, "true", str)
}
