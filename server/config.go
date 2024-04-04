package server

import (
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
