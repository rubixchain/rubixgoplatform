package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/did"
)

const (
	DIDUserTable string = "didusertable"
)

type DIDUserMap struct {
	ensweb.Base
	UserID string `gorm:"column:UserID"`
	DID    string `gorm:"column:DID;unique"`
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
	if s.cfg.EnableAuth {
		// always expect client tokne to present
		token := req.ClientToken.Model.(*Token)
		dm := DIDUserMap{
			Base: ensweb.Base{
				TenantID:             req.TenantID,
				CreationTime:         time.Now(),
				LastModificationTime: time.Now(),
			},
			UserID: token.UserID,
			DID:    did,
		}
		err = s.CreateEntity(DIDUserTable, &dm)
		if err != nil {
			s.BasicResponse(req, false, "Failed to update did user map", nil)
		}
	}
	return s.BasicResponse(req, true, "DID created successfully", &didResp)
}

// APIGetAllDID will get all DID
func (s *Server) APIGetAllDID(req *ensweb.Request) *ensweb.Result {
	var ids []string
	if s.cfg.EnableAuth {
		// always expect client tokne to present
		token := req.ClientToken.Model.(*Token)
		var err = s.GetEntity(DIDUserTable, req.TenantID, "UserID=?", &ids, token.UserID)
		if err != nil {
			ids = make([]string, 0)
		}
	} else {
		ids = s.c.GetAllDID()
	}
	return s.BasicResponse(req, true, "Got all DIDs", ids)
}

func (s *Server) validateDIDUserMapping(req *ensweb.Request, did string) bool {
	if s.cfg.EnableAuth {
		// always expect client tokne to present
		token := req.ClientToken.Model.(*Token)
		var dm DIDUserMap
		var err = s.GetEntity(DIDUserTable, req.TenantID, "UserID=? AND DID=?", &dm, token.UserID, did)
		if err != nil {
			return false
		} else {
			return true
		}
	} else {
		return true
	}
}
