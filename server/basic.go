package server

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// BasicResponse will send basic mode response
func (s *Server) BasicResponse(req *ensweb.Request, status bool, msg string, result interface{}) *ensweb.Result {
	resp := model.BasicResponse{
		Status:  status,
		Message: msg,
		Result:  result,
	}
	return s.RenderJSON(req, &resp, http.StatusOK)
}

// ShowAccount godoc
// @Summary      Start Core
// @Description  It will setup the core if not done before
// @Tags         Basic
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Router       /api/start [get]
func (s *Server) APIStart(req *ensweb.Request) *ensweb.Result {
	status, msg := s.c.Start()
	return s.BasicResponse(req, status, msg, nil)
}

// APIStart will setup the core
func (s *Server) APIShutdown(req *ensweb.Request) *ensweb.Result {
	go s.shutDown()
	s.c.ExpireUserAPIKey()
	return s.BasicResponse(req, true, "Shutting down...", nil)
}

// APIStart will setup the core
func (s *Server) APINodeStatus(req *ensweb.Request) *ensweb.Result {
	ok := s.c.NodeStatus()
	if ok {
		return s.BasicResponse(req, true, "Node is up and running", nil)
	} else {
		return s.BasicResponse(req, false, "Node is down, please check logs", nil)
	}
}

func (s *Server) shutDown() {
	s.log.Info("Shutting down...")
	time.Sleep(2 * time.Second)
	s.sc <- true
}

// APIPing will ping to given peer
func (s *Server) APIPing(req *ensweb.Request) *ensweb.Result {
	peerID := s.GetQuerry(req, "peerID")
	if peerID == "" {
		s.log.Error("PeerID cannot be empty")
		return s.BasicResponse(req, false, "PeerID cannot be empty", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(peerID)
	if !strings.HasPrefix(peerID, "12D3KooW") || len(peerID) != 52 || !is_alphanumeric {
		s.log.Error("Invalid PeerID")
		return s.BasicResponse(req, false, "Invalid PeerID", nil)
	}
	str, err := s.c.PingPeer(peerID)
	if err != nil {
		s.log.Error("ping failed", "err", err)
		return s.BasicResponse(req, false, str, nil)
	}
	return s.BasicResponse(req, true, str, nil)
}

// APIPing will ping to given peer
func (s *Server) APICheckQuorumStatus(req *ensweb.Request) *ensweb.Result {
	qAddress := s.GetQuerry(req, "quorumAddress")
	DID := qAddress
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(DID)
	if !strings.HasPrefix(DID, "bafybmi") || len(DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID of the quorum")
		return s.BasicResponse(req, false, "Invalid DID of the quorum", nil)
	}

	str, status, err := s.c.CheckQuorumStatus("", DID)
	if err != nil {
		s.log.Error("Quorum status check failed", "err", err)
		return s.BasicResponse(req, false, str, nil)
	}

	return s.BasicResponse(req, status, str, nil)
}
