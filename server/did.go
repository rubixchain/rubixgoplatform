package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	DIDRootDir string = "root"
)

// APICreateDID will create new DID
func (s *Server) APICreateDID(req *ensweb.Request) *ensweb.Result {

	folderName, err := s.c.CreateTempFolder()
	if err != nil {
		s.log.Error("failed to create folder")
		return s.BasicResponse(req, false, "failed to create folder", nil)
	}

	fileNames, fieldNames, err := s.ParseMultiPartForm(req, folderName+"/")

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

	if s.cfg.EnableAuth {
		// always expect client tokne to present
		token := req.ClientToken.Model.(*Token)
		didCreate.Dir = token.UserID
	} else {
		didCreate.Dir = DIDRootDir
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
	dir := DIDRootDir
	if s.cfg.EnableAuth {
		// always expect client token to present
		token := req.ClientToken.Model.(*Token)
		dir = token.UserID
	}
	dt := s.c.GetDIDs(dir)
	ai := model.GetAccountInfo{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got all DIDs",
		},
		AccountInfo: make([]model.DIDAccountInfo, 0),
	}
	for _, d := range dt {
		a, err := s.c.GetAccountInfo(d.DID)
		if err == nil {
			a.DIDType = d.Type
			ai.AccountInfo = append(ai.AccountInfo, a)
		}
	}
	return s.RenderJSON(req, &ai, http.StatusOK)
}

func (s *Server) validateDIDAccess(req *ensweb.Request, did string) bool {
	if s.cfg.EnableAuth {
		// always expect client token to present
		token := req.ClientToken.Model.(*Token)
		return s.c.IsDIDExist(token.UserID, did)
	} else {
		return true
	}
}
