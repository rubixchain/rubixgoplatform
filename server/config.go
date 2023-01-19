package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	SessionAuthMethod string = "SessionAuth"
	APIKeyAuthMethod  string = "APIKeyAuth"
)

type Config struct {
	config.Config
	EnableAuth  bool   `json:"enable_auth"`
	AuthMethod  string `json:"auth_method"`
	SessionName string `json:"session_name"`
	SessionKey  string `json:"session_key"`
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

// APICreateDID will create new DID
func (s *Server) APICreateDID(req *ensweb.Request) *ensweb.Result {

	folderName, err := s.c.CreateTempFolder()
	if err != nil {
		s.log.Error("failed to create folder")
		return s.BasicResponse(req, false, "failed to create folder", nil)
	}

	fileNames, fieldNames, err := s.ParseMultiPartForm(req, folderName+"/")

	fmt.Printf("Field : %v, Files : %v\n", fileNames, fieldNames)

	if err != nil {
		s.log.Error("failed to parse request", "err", err)
		return s.BasicResponse(req, false, "failed to create DID", nil)
	}
	fields := fieldNames[DIDConfigField]
	if len(fields) == 0 {
		s.log.Error("missing did configuration")
		return s.BasicResponse(req, false, "missing did configuration", nil)
	}
	var didCreate did.DIDCreate
	err = json.Unmarshal([]byte(fields[0]), &didCreate)
	if err != nil {
		s.log.Error("failed to parse did configuration", "err", err)
		return s.BasicResponse(req, false, "failed to parse did configuration", nil)
	}

	for _, fileName := range fileNames {
		if strings.Contains(fileName, did.ImgFileName) {
			didCreate.ImgFile = fileName
		}
		if strings.Contains(fileName, did.DIDImgFileName) {
			didCreate.DIDImgFile = fileName
		}
		if strings.Contains(fileName, did.PubShareFileName) {
			didCreate.PubImgFile = fileName
		}
		if strings.Contains(fileName, did.PubKeyFileName) {
			didCreate.PubKeyFile = fileName
		}
	}

	did, err := s.c.CreateDID(&didCreate)
	if err != nil {
		s.log.Error("failed to create did", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	didResp := DIDResponse{
		DID: did,
	}
	return s.BasicResponse(req, true, "DID created successfully", &didResp)
}

// APIGetAllDID will get all DID
func (s *Server) APIGetAllDID(req *ensweb.Request) *ensweb.Result {
	ids := s.c.GetAllDID()
	return s.BasicResponse(req, true, "Got all DIDs", ids)
}

// APIAddQuorum will add quorum list to node
func (s *Server) APIAddQuorum(req *ensweb.Request) *ensweb.Result {
	var ql model.QuorumList
	err := s.ParseJSON(req, &ql)
	if err != nil {
		return s.BasicResponse(req, false, "invlid input request", nil)
	}
	err = s.c.AddQuorum(&ql)
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
