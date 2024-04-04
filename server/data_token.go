package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ShowAccount godoc
// @Summary      Create Data Token
// @Description  This API will create data token
// @Tags         Data Tokens
// @Accept       mpfd
// @Produce      mpfd
// @Param        UserID      	   formData      string  false  "User/Entity ID"
// @Param        UserInfo      	   formData      string  false  "User/Entity Info"
// @Param        CommitterDID  	   formData      string  false  "Committer DID"
// @Param        BacthID    	   formData      string  false  "Batch ID"
// @Param        FileInfo    	   formData      string  false  "File Info is json string {"FileHash" : "asja", "FileURL": "ass", "FileTransInfo": "asas"}"
// @Param        FileContent       formData      file    false  "File to be committed"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/create-data-token [post]
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

// ShowAccount godoc
// @Summary      Create Data Token
// @Description  This API will create data token
// @Tags         Data Tokens
// @Accept       json
// @Produce      json
// @Param        did        	   query      string  true   "DID"
// @Param        batchID      	   query      string  false  "Batch ID"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/commit-data-token [post]
func (s *Server) APICommitDataToken(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	batchID := s.GetQuerry(req, "batchID")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	if batchID == "" {
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

// ShowAccount godoc
// @Summary      Get Data Token
// @Description  This API will get all data token belong to the did
// @Tags         Data Tokens
// @Accept       json
// @Produce      json
// @Param        did        	   query      string  true   "DID"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/get-data-token [get]
func (s *Server) APIGetDataToken(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		s.BasicResponse(req, false, "DID is required", nil)
	}
	dt := s.c.GetDataTokens(did)
	resp := model.DataTokenResponse{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Data tokens",
		},
		Tokens: dt,
	}
	return s.RenderJSON(req, resp, http.StatusOK)
}
