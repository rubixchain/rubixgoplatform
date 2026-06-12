package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	model "github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ShowAccount godoc
// @Summary      Create FT
// @Description  Mints a named fungible token (FT) for a DID, backed by the requested number of RBT tokens.
// @Tags         FT
// @Accept       json
// @Produce      json
// @Param        input body types.CreateFTReq true "Create FT"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/fts/mint [post]
func (s *Server) APICreateFT(req *ensweb.Request) *ensweb.Result {
	var createFTReq types.CreateFTReq
	err := s.ParseJSON(req, &createFTReq)
	if err != nil {
		return s.BasicResponse(req, false, "APICreateFT: Invalid input", nil)
	}
	if !s.validateDIDAccess(req, createFTReq.DID) {
		return s.BasicResponse(req, false, "APICreateFT: DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.CreateFTs(req.ID, createFTReq)
	return s.didResponse(req, req.ID)
}

// GetFTBalance godoc
// @Summary      Get FT Balance
// @Description  Retrieves the Fungible Token (FT) balance for a given DID.
// @Tags         DID
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
		Result:  ftInfo,
	}
	if len(ftInfo) == 0 {
		ac.Message = "No FTs found"
	}
	return s.RenderJSON(req, ac, http.StatusOK)
}

// ShowAccount godoc
// @Summary      List FTs
// @Description  Returns all fungible tokens (FTs) held on this node.
// @Tags         FT
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
