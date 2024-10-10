package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type CreateFTReqSwaggoInput struct {
	DID        string  `json:"did"`
	FTName     string  `json:"ftname"`
	FTCount    int     `json:"ftcount"`
	TokenCount float64 `json:"tokencount"`
}

type TransferFTReqSwaggoInput struct {
	Receiver string `json:"receiver"`
	Sender   string `json:"sender"`
	FTName   string `json:"FTName"`
	FTCount  int    `json:"FTCount"`
	Comment  string `json:"comment"`
	Type     int    `json:"type"`
	Password string `json:"password"`
}

// ShowAccount godoc
// @Summary      Create FT
// @Description  This API endpoint will create FTs.
// @Tags         FT
// @Accept       json
// @Produce      json
// @Param        input body CreateFTReqSwaggoInput true "Create FT"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/create-ft [post]
func (s *Server) APICreateFT(req *ensweb.Request) *ensweb.Result {
	var createFTReq model.CreateFTReq
	err := s.ParseJSON(req, &createFTReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if !s.validateDIDAccess(req, createFTReq.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.CreateFTs(req.ID, createFTReq.DID, createFTReq.FTCount, createFTReq.FTName, createFTReq.TokenCount)
	return s.didResponse(req, req.ID)
}

// ShowAccount godoc
// @Summary      Initiate FT transfer
// @Description  This API endpoint will do transfer of FTs.
// @Tags         FT
// @Accept       json
// @Produce      json
// @Param        input body TransferFTReqSwaggoInput true "Transfer FT"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/initiate-ft-tranfer [post]
func (s *Server) APIInitiateFTTransfer(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.TransferFTReq
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(rbtReq.Sender)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateFTTransfer(req.ID, &rbtReq)
	return s.didResponse(req, req.ID)
}

func (s *Server) APIGetFTInfo(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	info, err := s.c.GetFTInfo(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	ac := model.GetFTInfo{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got FT info successfully",
		},
		FTInfo: make([]model.FTInfo, 0),
	}
	ac.FTInfo = append(ac.FTInfo, info...)
	return s.RenderJSON(req, ac, http.StatusOK)
}
