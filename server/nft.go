package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
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
	if nft == "" {
		return s.BasicResponse(request, false, "NFT query parameter is required", nil)
	}

	// Validate NFT format
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(nft)
	if len(nft) != 46 || !strings.HasPrefix(nft, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid NFT")
		return s.BasicResponse(request, false, "Invalid NFT", nil)
	}

	topic := nft
	s.c.AddWebReq(request)
	go s.c.SubscribeNFTSetup(request.ID, topic)
	return s.BasicResponse(request, true, "NFT subscribed successfully", nil)
}

// NFT godoc
// @Summary      Fetch NFT
// @Description  This API will Fetch NFT
// @Tags         NFT
// @ID   	     fetch-nft
// @Accept       json
// @Produce      json
// @Param        input query NewNFTSwaggoInput true "Fetch nft"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/fetch-nft [get]
func (s *Server) APIFetchNft(req *ensweb.Request) *ensweb.Result {
	var fetchNft core.FetchNFTRequest
	var err error

	// Get the NFT id from the request
	fetchNft.NFT = s.GetQuery(req, "nft")

	// Validate the NFT id
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(fetchNft.NFT)
	if len(fetchNft.NFT) != 46 || !strings.HasPrefix(fetchNft.NFT, "Qm") || !isAlphanumeric {
		s.log.Error("Invalid nft")
		return s.BasicResponse(req, false, "Invalid nft", nil)
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

// ShowAccount godoc
// @Summary      Get NFTs owned by the particular did
// @Description  This API will get all NFTs owned by the particular did
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        input query GetNFTSwaggoInput true "Get nfts by did"
// @Success      200  {object}  model.NFTList
// @Router       /api/get-nfts-by-did [get]
func (s *Server) APIGetNFTsByDid(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuery(req, "did")
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
