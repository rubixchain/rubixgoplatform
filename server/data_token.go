package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core"
)

func (s *Server) APICreateDataToken(req *ensweb.Request) *ensweb.Result {
	var dr core.DataTokenReq
	var err error
	dr.DID = s.GetQuerry(req, "did")
	dr.FolderName, err = s.c.CreateTempFolder()
	if err != nil {
		s.log.Error("failed to create folder", "err", err)
		return s.BasicResponse(req, false, "failed to create folder", nil)
	}
	dr.FileNames, dr.Fields, err = s.ParseMultiPartForm(req, dr.FolderName+"/")

	if err != nil {
		s.log.Error("Create data token failed, failed to parse request", "err", err)
		return s.BasicResponse(req, false, "Create data token failed, failed to parse request", nil)
	}

	if !s.validateDIDAccess(req, dr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.CreateDataToken(req.ID, &dr)
	return s.didResponse(req, req.ID)

}

func (s *Server) APICommitDataToken(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	batchID := s.GetQuerry(req, "batchID")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	if batchID == "batchID1" {
		batchID = did
	}
	s.c.AddWebReq(req)
	go s.c.CommitDataToken(req.ID, did, batchID)
	return s.didResponse(req, req.ID)
}

func (s *Server) APICheckDataToken(req *ensweb.Request) *ensweb.Result {
	dt := s.GetQuerry(req, "data_token")
	if dt == "" {
		s.BasicResponse(req, false, "Data token required", nil)
	}
	ok := s.c.CheckDataToken(dt)
	if !ok {
		s.BasicResponse(req, false, "Data token is invalid", nil)
	}
	return s.BasicResponse(req, true, "Data token is valid", nil)
}
