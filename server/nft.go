package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core"
	model "github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// NFT godoc
// @Summary      Create NFT
// @Description  This API will create new NFT
// @Tags         NFT
// @Accept       mpfd
// @Produce      mpfd
// @Param        did        	   formData      string  true   "DID"
// @Param        metadata       formData      file  true  "JSON file which contains information about the NFT"
// @Param        artifact       formData      file    true  "File which is meant to be an NFT"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/nfts/generate [post]
func (s *Server) APICreateNFT(req *ensweb.Request) *ensweb.Result {
	var createNFT core.NFTReq
	var err error
	createNFT.NFTPath, err = s.c.CreateNFTTempFolder()
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to create NFT folder", "err", err)
		return s.BasicResponse(req, false, "Failed to create NFT, Failed to create NFT folder", nil)
	}
	nftInfoFile, nftInfoFileHeader, err := s.ParseMultiPartFormFile(req, "metadata")
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to retrieve metadata", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to retrieve metadata", nil)
	}
	nftFileInfoDest := filepath.Join(createNFT.NFTPath, nftInfoFileHeader.Filename)
	nftFileInfoDestFile, err := os.Create(nftFileInfoDest)
	if err != nil {
		nftInfoFile.Close()
		s.log.Error("Creation of NFT failed, failed to write NFT file Info", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to write NFT file Info", nil)
	}

	nftInfoFile.Close()
	nftFileInfoDestFile.Close()

	err = moveFile(nftInfoFile.Name(), nftFileInfoDest)
	if err != nil {
		nftInfoFile.Close()
		s.log.Error("Creation of NFT failed, failed to move NFT file Info", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to move NFTFile", nil)
	}

	nftFile, nftFileHeader, err := s.ParseMultiPartFormFile(req, "artifact")
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to retrieve NFT artifact", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to retrieve NFT artifact", nil)
	}
	nftFileDest := filepath.Join(createNFT.NFTPath, nftFileHeader.Filename)
	nftFileDestFile, err := os.Create(nftFileDest)
	if err != nil {
		nftFileInfoDestFile.Close()
		nftFileDestFile.Close()
		s.log.Error("Creation of NFT failed, failed to write NFT file", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to write NFT file", nil)
	}
	nftFile.Close()
	nftFileDestFile.Close()
	err = moveFile(nftFile.Name(), nftFileDest)
	if err != nil {
		nftFileInfoDestFile.Close()
		nftFileDestFile.Close()
		s.log.Error("Create NFT failed, failed to move NFT file", "err", err)
		return s.BasicResponse(req, false, "Create NFT failed, failed to move NFT file", nil)
	}

	createNFT.Artifact = nftFileDest
	createNFT.Metadata = nftFileInfoDest

	_, did, err := s.ParseMultiPartForm(req, "did")
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to retrieve DID", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to retrieve DID", nil)
	}
	createNFT.DID = did["did"][0]
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(createNFT.DID)
	if !strings.HasPrefix(createNFT.DID, "bafybmi") || len(createNFT.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}

	if !s.validateDIDAccess(req, createNFT.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.CreateNFTRequest(req.ID, createNFT)
	return s.didResponse(req, req.ID)

}

// NFT godoc
// @Summary      Subscribe to NFT
// @Description  This API endpoint allows subscribing to a NFT.
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        nft query string true "NFT token to subscribe to"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/nfts/subscribe [get]
func (s *Server) APISubscribeNFT(request *ensweb.Request) *ensweb.Result {
	// Get NFT from query parameter
	nft := s.GetQuery(request, "nft")
	if err := util.ValidateCIDFormat(nft); err != nil {
		s.log.Error("Invalid NFT", "err", err)
		return s.BasicResponse(request, false, fmt.Sprintf("NFT %s", err.Error()), nil)
	}

	topic := nft
	s.c.AddWebReq(request)
	go s.c.SubscribeNFTSetup(request.ID, topic)
	return s.BasicResponse(request, true, "NFT subscribed successfully", nil)
}

func (s *Server) APIFetchNft(req *ensweb.Request) *ensweb.Result {
	var fetchNft core.FetchNFTRequest
	var err error

	// Get the NFT id from the request
	fetchNft.NFT = s.GetQuery(req, "nft")

	if err := util.ValidateCIDFormat(fetchNft.NFT); err != nil {
		s.log.Error("Invalid nft", "err", err)
		return s.BasicResponse(req, false, fmt.Sprintf("NFT %s", err.Error()), nil)
	}

	// Check if the NFT directory already exists
	existingPath, err := s.c.CheckNFTFolderExists(fetchNft.NFT)
	if err != nil {
		s.log.Error("Failed to check if NFT folder exists", "err", err)
		return s.BasicResponse(req, false, "Failed to check NFT folder", nil)
	}
	if existingPath != "" {
		s.log.Debug("NFT directory already exists")
		return s.BasicResponse(req, true, "NFT directory already exists", nil)
	}

	// Create a temporary NFT folder
	fetchNft.NFTPath, err = s.c.CreateNFTTempFolder()
	if err != nil {
		s.log.Error("Fetch nft failed, failed to create nft folder", "err", err)
		return s.BasicResponse(req, false, "Fetch nft failed, failed to create nft folder", nil)
	}

	// Rename the temporary folder to the NFT name
	fetchNft.NFTPath, err = s.c.RenameNFTFolder(fetchNft.NFTPath, fetchNft.NFT)
	if err != nil {
		s.log.Error("Fetch nft failed, failed to rename nft folder", "err", err)
		return s.BasicResponse(req, false, "Fetch nft failed, failed to rename nft folder", nil)
	}

	// Fetch the NFT
	basicResponse := s.c.FetchNFT(&fetchNft)
	if !basicResponse.Status {
		s.log.Error("Fetch nft failed", "err", basicResponse.Message)
		return s.BasicResponse(req, false, basicResponse.Message, nil)
	}

	return s.BasicResponse(req, basicResponse.Status, basicResponse.Message, nil)
}

// NFT godoc
// @Summary      Get all nfts
// @Description  This API will return all nfts
// @Tags         NFT
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/nfts [get]

func (s *Server) APIListNFTs(req *ensweb.Request) *ensweb.Result {
	response, err := s.c.GetAllNFTs()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to retrieve nfts", nil)
	}
	return s.BasicResponse(req, true, "Nfts retrieved successfully", response)
}

// NFT godoc
// @Summary      Get nft chain by token ID
// @Description  This API will return the nft chain for a given nft token ID
// @Tags         NFT
// @Produce      json
// @Param        nft_id   path      string  true  "NFT Token ID"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/nfts/{nft_id}/chain [get]
func (s *Server) APIGetNFTChain(req *ensweb.Request) *ensweb.Result {
	nftID := s.GetRouteVar(req, "nft_id")
	TokenChainResponse, err := s.c.GetNFTChain(nftID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to retrieve nft chain data", nil)
	}
	return s.BasicResponse(req, true, "Nft chain data retrieved successfully", TokenChainResponse)
}

// APIGetChildNFTs godoc
// @Summary      Get child NFTs of a given parent NFT
// @Description  Returns NFTs whose parent_token_id matches the given parent NFT ID. Originator-only.
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        nft_id  path      string  true  "Parent NFT token ID"
// @Success      200     {object}  model.BasicResponse
// @Failure      400     {object}  model.BasicResponse
// @Router       /rubix/v1/nfts/{nft_id}/children [get]
func (s *Server) APIGetChildNFTs(req *ensweb.Request) *ensweb.Result {
	parentNFTId := s.GetRouteVar(req, "nft_id")
	if err := util.ValidateCIDFormat(parentNFTId); err != nil {
		return s.BasicResponse(req, false, fmt.Sprintf("parent NFT %s", err.Error()), nil)
	}
	children, err := s.c.GetChildNFTs(parentNFTId)
	if err != nil {
		return s.BasicResponse(req, false, "failed to get child NFTs, err: "+err.Error(), nil)
	}
	message := "got child NFTs successfully"
	if len(children) == 0 {
		message = "no child NFTs found"
	}
	resp := model.BasicResponse{
		Status:  true,
		Message: message,
		Result:  children,
	}
	return s.RenderJSON(req, resp, http.StatusOK)
}

// APIGetParentNFT godoc
// @Summary      Get the parent NFT of a given child NFT
// @Description  Returns the parent NFT (id + value) of the named NFT, or null when there is no parent. Originator-only.
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        nft_id  path      string  true  "Child NFT token ID"
// @Success      200     {object}  model.BasicResponse
// @Failure      400     {object}  model.BasicResponse
// @Router       /rubix/v1/nfts/{nft_id}/parent [get]
func (s *Server) APIGetParentNFT(req *ensweb.Request) *ensweb.Result {
	childNFTId := s.GetRouteVar(req, "nft_id")
	if err := util.ValidateCIDFormat(childNFTId); err != nil {
		return s.BasicResponse(req, false, fmt.Sprintf("child NFT %s", err.Error()), nil)
	}
	parent, err := s.c.GetParentNFT(childNFTId)
	if err != nil {
		return s.BasicResponse(req, false, "failed to get parent NFT, err: "+err.Error(), nil)
	}
	message := "got parent NFT successfully"
	if parent == nil {
		message = "no parent NFT"
	}
	resp := model.BasicResponse{
		Status:  true,
		Message: message,
		Result:  parent,
	}
	return s.RenderJSON(req, resp, http.StatusOK)
}

// ShowAccount godoc
// @Summary      Get NFTs owned by the particular did
// @Description  This API will get all NFTs owned by the particular did
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID (e.g. did:bafybmih3l2emb4s7wbsgakwv4voaqngdirpg5f3kqlheqqsgdg7jthuwaq)"
// @Success      200  {object}  model.BasicResponse
// @Failure      400  {object}  model.BasicResponse
// @Router       /rubix/v1/dids/{did}/balances/nft [get]
func (s *Server) APIGetNFTsByDid(req *ensweb.Request) *ensweb.Result {
	did := s.GetRouteVar(req, "did")
	resp, err := s.c.GetNFTsByDid(did)
	if err != nil {
		return s.BasicResponse(req, false, "failed to get nfts, err: "+err.Error(), nil)
	}
	nftResp := model.BasicResponse{
		Status:  true,
		Message: "got NFTs balance successfully",
		Result:  resp,
	}
	return s.RenderJSON(req, nftResp, http.StatusOK)
}

// APIGetNFTProperties godoc
// @Summary      Get the properties governing an NFT
// @Description  Returns the resolved permission document, or reports that the NFT is unrestricted.
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        nft_id  path      string  true  "NFT token ID"
// @Success      200     {object}  model.BasicResponse
// @Failure      400     {object}  model.BasicResponse
// @Router       /rubix/v1/nfts/{nft_id}/properties [get]
func (s *Server) APIGetNFTProperties(req *ensweb.Request) *ensweb.Result {
	nftID := s.GetRouteVar(req, "nft_id")
	if err := util.ValidateCIDFormat(nftID); err != nil {
		return s.BasicResponse(req, false, fmt.Sprintf("NFT %s", err.Error()), nil)
	}

	resolved, err := s.c.GetNFTProperties(nftID)
	if err != nil {
		return s.BasicResponse(req, false, "failed to get NFT properties, err: "+err.Error(), nil)
	}
	if resolved == nil || resolved.Doc == nil {
		return s.BasicResponse(req, true, "NFT has no properties and is unrestricted", nil)
	}

	return s.BasicResponse(req, true, "got NFT properties successfully", model.NFTPropertiesResponse{
		NFTId:                 nftID,
		PropertiesTokenID:     resolved.PropertiesTokenID,
		PropertiesCID:         resolved.DocCID,
		Version:               resolved.Doc.Version,
		Transferable:          resolved.Doc.IsTransferable(),
		ValidFrom:             resolved.Doc.Policy.ValidFrom,
		ValidTo:               resolved.Doc.Policy.ValidTo,
		Whitelist:             resolved.Whitelist,
		Admins:                resolved.Admins,
		AllowedSubnets:        resolved.Doc.Restriction.AllowedSubnets,
		AllowedSmartContracts: resolved.Doc.Restriction.AllowedSmartContracts,
		Deployer:              resolved.Deployer,
	})
}
