package server

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type CreateFTReqSwaggoInput struct {
	DID             string  `json:"did" swaggo:"true,The DID of the user"`
	FTName          string  `json:"ft_name" swaggo:"true,The name of the FT"`
	FTCount         int     `json:"ft_count" swaggo:"true,The number of FTs to create"`
	FTValue         float64 `json:"ft_value" swaggo:"true,The value of each FT to create"`
	FTNumStartIndex int     `json:"ft_num_start_index" swaggo:"true,The starting index for FT numbering"`
	FromRBT         bool    `json:"from_rbt" swaggo:"false,Whether to create FT from RBT,default=false"`
}

type TransferFTReqSwaggoInput struct {
	Receiver   string `json:"receiver"`
	Sender     string `json:"sender"`
	FTName     string `json:"ft_name"`
	FTCount    int    `json:"ft_count"`
	Comment    string `json:"comment"`
	QuorumType int    `json:"quorum_type"`
	Password   string `json:"password"`
	CreatorDID string `json:"creatorDID"`
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
	// High Value true hardcoded for easy testing.
	go s.c.CreateFTs(req.ID, createFTReq.DID, createFTReq.FTCount, createFTReq.FTName, createFTReq.FTValue, createFTReq.FTNumStartIndex, createFTReq.FromRBT, true)
	return s.didResponse(req, req.ID)
}

// ShowAccount godoc
// @Summary      Initiate an FT transfer
// @Description  This API endpoint will initiate transfer of FTs.
// @Tags         FT
// @Accept       json
// @Produce      json
// @Param        input body TransferFTReqSwaggoInput true "Transfer FT"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/initiate-ft-transfer [post]
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

// ShowAccount godoc
// @Summary      Get FT balance information for a given DID
// @Description  This API endpoint retrieves the names and count of FTs of a given DID.
// @Tags         FT
// @Accept       json
// @Produce      json
// @Param        did      	   query      string  true  "User DID"
// @Success      200  {object}  model.GetFTInfo
// @Router       /api/get-ft-info-by-did [get]
func (s *Server) APIGetFTInfo(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have access", nil)
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !isAlphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	info, err := s.c.GetFTInfoByDID(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	ac := model.GetFTInfo{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got FT info successfully",
		},
		FTInfo: info,
	}
	if len(info) == 0 {
		ac.Message = "No FTs found"
	}
	return s.RenderJSON(req, ac, http.StatusOK)
}

func (s *Server) APIBurnFT(req *ensweb.Request) *ensweb.Result {
	var burnFTReq model.BurnFTReq
	err := s.ParseJSON(req, &burnFTReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if !s.validateDIDAccess(req, burnFTReq.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.BurnFT(req.ID, &burnFTReq)
	return s.didResponse(req, req.ID)
}
