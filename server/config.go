package server

import (
	"fmt"
	"strings"

	model "github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const (
	SessionAuthMethod string = "SessionAuth"
	APIKeyAuthMethod  string = "APIKeyAuth"
	BasicAuthMethod   string = "BasicAuth"
)

type Config struct {
	config.Config
	EnableAuth  bool   `json:"enable_auth"`
	APIKey      string `json:"api_key"`
	AuthMethod  string `json:"auth_method"`
	SessionName string `json:"session_name"`
	SessionKey  string `json:"session_key"`
	GRPCAddr    string `json:"grpc_addr"`
	GRPCSecure  bool   `json:"grpc_secure"`
}

// APIAddBootStrap will add bootstrap peers to the configuration
func (s *Server) APIAddBootStrap(req *ensweb.Request) *ensweb.Result {
	var m model.BootStrapPeers
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "invlid input request", nil)
	}
	if len(m.Peers) == 0 {
		s.log.Error("bootstrap Peers required to add")
		return s.BasicResponse(req, false, "Bootstrap Peers required to add", nil)
	}
	for _, peer := range m.Peers {
		if !strings.HasPrefix(peer, "/") {
			s.log.Error(fmt.Sprintf("Invalid bootstrap peer : %s", peer))
			return s.BasicResponse(req, false, "Invalid bootstrap peer", nil)
		}
	}

	err = s.c.AddBootStrap(m.Peers)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to add bootstrap peers, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Boostrap peers added successfully", nil)
}

// APIRemoveBootStrap will remove bootstrap peers from the configuration
func (s *Server) APIRemoveBootStrap(req *ensweb.Request) *ensweb.Result {
	var m model.BootStrapPeers
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "invlid input request", nil)
	}
	if len(m.Peers) == 0 {
		s.log.Error("Bootstrap peers required to remove")
		return s.BasicResponse(req, false, "Bootstrap peers required to remove", nil)
	}
	for _, peer := range m.Peers {
		if !strings.HasPrefix(peer, "/") {
			s.log.Error(fmt.Sprintf("Invalid bootstrap peer : %s", peer))
			return s.BasicResponse(req, false, "Invalid bootstrap peer", nil)
		}
	}
	err = s.c.RemoveBootStrap(m.Peers)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to remove bootstrap peers, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Boostrap peers removed successfully", nil)
}

// APIRemoveAllBootStrap will remove all bootstrap peers from the configuration
func (s *Server) APIRemoveAllBootStrap(req *ensweb.Request) *ensweb.Result {
	err := s.c.RemoveAllBootStrap()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to remove all bootstrap peers, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "All boostrap peers removed successfully", nil)
}

// APIRemoveAllBootStrap will remove all bootstrap peers from the configuration
func (s *Server) APIGetAllBootStrap(req *ensweb.Request) *ensweb.Result {
	peers := s.c.GetAllBootStrap()
	m := model.BootStrapPeers{
		Peers: peers,
	}
	return s.BasicResponse(req, true, "Got all the bootstrap peers successfully", m)
}

// APIAddQuorum godoc
// @Summary      Add quorum
// @Description  Adds a quorum to the node's quorum list, identified by its DID.
// @Tags         Quorum
// @Accept       json
// @Produce      json
// @Param        input  body      model.AddQuorumRequest  true  "Quorum to add"
// @Success      200    {object}  model.BasicResponse
// @Router       /api/addquorum [post]
func (s *Server) APIAddQuorum(req *ensweb.Request) *ensweb.Result {
	var reqBody model.AddQuorumRequest
	err := s.ParseJSON(req, &reqBody)
	if err != nil {
		return s.BasicResponse(req, false, "invalid input request", nil)
	}
	ql := strings.TrimSpace(reqBody.DID)
	if ql == "" {
		return s.BasicResponse(req, false, "did is required", nil)
	}
	err = s.c.AddQuorum(ql)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to add quorums, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Quorums added successfully", nil)
}

// APIGetAllQuorum godoc
// @Summary      Get all quorums
// @Description  Returns the list of quorums configured on the node.
// @Tags         Quorum
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Router       /api/getallquorum [get]
func (s *Server) APIGetAllQuorum(req *ensweb.Request) *ensweb.Result {
	ql, err := s.c.GetAllQuorum()
	if err != nil {
		return s.BasicResponse(req, false, fmt.Sprintf("Failed to get quorums, err: %v ", err.Error()), nil)
	}
	return s.BasicResponse(req, true, "Got all quorums successfully", ql)
}

// APIRemoveAllQuorum godoc
// @Summary      Remove all quorums
// @Description  Removes all quorums from the node's quorum list.
// @Tags         Quorum
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Router       /api/removeallquorum [get]
func (s *Server) APIRemoveAllQuorum(req *ensweb.Request) *ensweb.Result {
	err := s.c.RemoveAllQuorum()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to remove all quorums", nil)
	}
	return s.BasicResponse(req, true, "Removed all quorums successfully", nil)
}
