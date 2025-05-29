package server

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core"
	cc "github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/model"
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

// APIAddQuorum will add quorum list to node
func (s *Server) APIAddQuorum(req *ensweb.Request) *ensweb.Result {
	var ql []core.QuorumData
	err := s.ParseJSON(req, &ql)
	if err != nil {
		return s.BasicResponse(req, false, "invlid input request", nil)
	}
	if len(ql) < 5 {
		s.log.Error("Length of Quorum list should be atleast 5")
		return s.BasicResponse(req, false, "Length of Quorum list should be atleast 5", nil)
	}
	for _, q := range ql {
		is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(q.Address)
		if !strings.HasPrefix(q.Address, "bafybmi") || len(q.Address) != 59 || !is_alphanumeric {
			s.log.Error(fmt.Sprintf("Invalid quorum DID : %s", q.Address))
			return s.BasicResponse(req, false, fmt.Sprintf("Invalid quorum DID : %s", q.Address), nil)
		}
	}
	err = s.c.AddQuorum(ql)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to add quorums, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Quorums added successfully", nil)
}

// APIGetAllQuorum will get quorum list from node
func (s *Server) APIGetAllQuorum(req *ensweb.Request) *ensweb.Result {
	ql := s.c.GetAllQuorum()
	return s.BasicResponse(req, true, "Got all quorums successfully", ql)
}

// APIRemoveAllQuorum will remove quorum list from node
func (s *Server) APIRemoveAllQuorum(req *ensweb.Request) *ensweb.Result {
	err := s.c.RemoveAllQuorum()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to remove all quorums", nil)
	}
	return s.BasicResponse(req, true, "Removed all quorums successfully", nil)
}

func (s *Server) APISetupDB(req *ensweb.Request) *ensweb.Result {
	var sc cc.StorageConfig
	err := s.ParseJSON(req, &sc)
	if err != nil {
		return s.BasicResponse(req, false, "invlid input request", nil)
	}
	err = s.c.SetupDB(&sc)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to setup DB, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "DB setup done successfully", nil)
}

// APIGetAllExplorer will get all explorer URLs from the db
func (s *Server) APIGetAllExplorer(req *ensweb.Request) *ensweb.Result {
	links, err := s.c.GetAllExplorer()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get explorer urls"+err.Error(), nil)
	}
	m := model.ExplorerLinks{
		Links: links,
	}
	return s.BasicResponse(req, true, "Got all the explorer URLs successfully", m)
}

// APIAddPeerDetailsFromExplorer will add peer details from explorer
func (s *Server) APIAddPeerDetailsFromExplorer(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		s.log.Error("DID cannot be empty")
		return s.BasicResponse(req, false, "DID cannot be empty", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	_, err := s.c.GetPeerDIDInfo(did)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to add peer details from explorer, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Peer details added successfully", nil)
}

// APIAddExplorer will add bootstrap peers to the configuration
func (s *Server) APIAddExplorer(req *ensweb.Request) *ensweb.Result {
	var m model.ExplorerLinks
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "invlid input request", nil)
	}
	if len(m.Links) == 0 {
		s.log.Error("explorer links required to add")
		return s.BasicResponse(req, false, "explorer links required to add", nil)
	}
	err = s.c.AddExplorer(m.Links)
	if err != nil {
		return s.BasicResponse(req, false, "failed to add explorer, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "explorer added successfully", nil)
}

// APIRemoveExplorer will remove bootstrap peers from the configuration
func (s *Server) APIRemoveExplorer(req *ensweb.Request) *ensweb.Result {
	var m model.ExplorerLinks
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "invlid input request", nil)
	}
	if len(m.Links) == 0 {
		s.log.Error("explorer links required to remove")
		return s.BasicResponse(req, false, "explorer links required to remove", nil)
	}
	err = s.c.RemoveExplorer(m.Links)
	if err != nil {
		return s.BasicResponse(req, false, "failed to remove explorer, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "explorer removed successfully", nil)
}

func (s *Server) APIAddUserAPIKey(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	apiKey := s.GetQuerry(req, "apiKey")

	err := s.c.AddDIDKey(did, apiKey)
	if err != nil {
		return s.BasicResponse(req, false, "failed to add to table, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Api Key added successfully", nil)
}
