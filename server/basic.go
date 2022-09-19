package server

import (
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
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
