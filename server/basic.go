package server

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	model "github.com/rubixchain/rubixgoplatform/types/models"
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

// APIPing godoc
// @Summary      Ping a peer
// @Description  Pings the given peer by peerID to check reachability.
// @Tags         Peer
// @Produce      json
// @Param        peerID  query     string  true  "Peer ID of the node to ping (52-char alphanumeric, prefixed with 12D3KooW)"
// @Success      200     {object}  model.BasicResponse
// @Router       /api/ping [get]
func (s *Server) APIPing(req *ensweb.Request) *ensweb.Result {
	peerID := s.GetQuery(req, "peerID")
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

// APICheckQuorumStatus godoc
// @Summary      Check quorum status
// @Description  Checks whether the quorum identified by the given DID is set up and available.
// @Tags         Quorum
// @Produce      json
// @Param        quorumAddress  query     string  true  "DID of the quorum (59-char alphanumeric, prefixed with bafybmi)"
// @Success      200            {object}  model.BasicResponse
// @Router       /api/check-quorum-status [get]
func (s *Server) APICheckQuorumStatus(req *ensweb.Request) *ensweb.Result {
	qAddress := s.GetQuery(req, "quorumAddress")
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
