package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type CreateFTReqSwaggoInput struct {
	DID             string `json:"did"`
	FTName          string `json:"ft_name"`
	FTCount         int    `json:"ft_count"`
	TokenCount      int    `json:"token_count"`
	FTNumStartIndex int    `json:"ft_num_start_index"`
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
// @Router       /rubix/v1/fts/mint [post]
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
	rbtAmount := int(createFTReq.TokenCount)
	go s.c.CreateFTs(req.ID, createFTReq.DID, createFTReq.FTCount, createFTReq.FTName, rbtAmount, createFTReq.FTNumStartIndex)
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

// GetFTBalance godoc
// @Summary      Get FT Balance
// @Description  Retrieves the Fungible Token (FT) balance for a given DID.
// @Tags         dids
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID (e.g. did:bafybmih3l2emb4s7wbsgakwv4voaqngdirpg5f3kqlheqqsgdg7jthuwaq)"
// @Success      200  {object}  model.BasicResponse
// @Failure      400  {object}  model.BasicResponse
// @Router       /rubix/v1/dids/{did}/balances/ft [get]
func (s *Server) APIGetFTInfo(req *ensweb.Request) *ensweb.Result {
	did := s.GetRouteVar(req, "did")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have access", nil)
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !isAlphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	ftInfo, err := s.c.GetFTInfoByDID(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	ac := model.BasicResponse{
		Status:  true,
		Message: "Got FT info successfully",
		Result: ftInfo,
	}
	if len(ftInfo) == 0 {
		ac.Message = "No FTs found"
	}
	return s.RenderJSON(req, ac, http.StatusOK)
}

func (s *Server) APIFixFTCreator(req *ensweb.Request) *ensweb.Result {
	s.log.Info("Fixing FT tokens with peer ID as CreatorDID")

	// Run the fix utility
	results, err := s.c.FixAllFTTokensWithPeerIDAsCreator()
	if err != nil {
		s.log.Error("Failed to fix FT creators", "err", err)
		return s.BasicResponse(req, false, "Failed to fix FT creators: "+err.Error(), nil)
	}

	// Convert results to a format suitable for JSON response
	var responseResults []map[string]interface{}
	successCount := 0
	for _, result := range results {
		r := map[string]interface{}{
			"TokenID":    result.TokenID,
			"OldCreator": result.OldCreator,
			"NewCreator": result.NewCreator,
			"Success":    result.Success,
		}
		if result.Error != nil {
			r["Error"] = result.Error.Error()
		}
		if result.Success {
			successCount++
		}
		responseResults = append(responseResults, r)
	}

	message := fmt.Sprintf("Fixed %d of %d FT tokens", successCount, len(results))
	if len(results) == 0 {
		message = "No FT tokens found with peer ID as CreatorDID"
	}

	return s.BasicResponse(req, true, message, responseResults)
}

func (s *Server) APIGetFTCreatorStats(req *ensweb.Request) *ensweb.Result {
	s.log.Info("Getting FT creator statistics")

	// Get the statistics
	stats, err := s.c.GetFTTokenCreatorStats()
	if err != nil {
		s.log.Error("Failed to get FT creator stats", "err", err)
		return s.BasicResponse(req, false, "Failed to get FT creator statistics: "+err.Error(), nil)
	}

	return s.BasicResponse(req, true, "FT creator statistics retrieved successfully", stats)
}

// ShowAccount godoc
// @Summary      List FTs
// @Description  This API endpoint will list FTs.
// @Tags         fts
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/fts [get]
func (s *Server) APIListFTs(req *ensweb.Request) *ensweb.Result {
	ftList, err := s.c.ListFTs()
	if err != nil {
		return s.BasicResponse(req, false, fmt.Sprintf("failed to get the list of FTs, err: %v", err), nil)
	}

	return s.BasicResponse(req, true, "FT Fetched successfully", ftList)
}
