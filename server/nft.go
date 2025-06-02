package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
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
// @Router       /api/create-nft [post]
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

type DeployNFTSwaggoInput struct {
	NFT        string  `json:"nft"`
	DID        string  `json:"did"`
	QuorumType int     `json:"quorum_type"`
	NFTValue   float64 `json:"nft_value"`
	NFTData    string  `json:"nft_data"`
}

// NFT godoc
// @Summary      Deploy NFT
// @Description  This API will deploy the NFT
// @Tags         NFT
// @ID 			 deploy-nft
// @Accept       json
// @Produce      json
// @Param		 input body DeployNFTSwaggoInput true "Deploy nft"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/deploy-nft [post]
func (s *Server) APIDeployNFT(req *ensweb.Request) *ensweb.Result {
	var deployReq model.DeployNFTRequest
	err := s.ParseJSON(req, &deployReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(deployReq.NFT)
	if len(deployReq.NFT) != 46 || !strings.HasPrefix(deployReq.NFT, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid smart contract token")
		return s.BasicResponse(req, false, "Invalid smart contract token", nil)
	}
	_, did, ok := util.ParseAddress(deployReq.DID)
	if !ok {
		return s.BasicResponse(req, false, "Invalid Deployer address", nil)
	}

	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid deployer DID")
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if deployReq.QuorumType < 1 || deployReq.QuorumType > 2 {
		s.log.Error("Invalid quorum type")
		return s.BasicResponse(req, false, "Invalid quorum type", nil)
	}

	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.DeployNFT(req.ID, deployReq)
	return s.didResponse(req, req.ID)
}

// ShowAccount godoc
// @Summary      Get ALL NFTs
// @Description  This API will get all NFTs deployed on the node
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.NFTList
// @Router       /api/list-nfts [get]
func (s *Server) APIGetAllNFT(req *ensweb.Request) *ensweb.Result {
	resp := s.c.GetAllNFT()
	return s.RenderJSON(req, resp, http.StatusOK)
}

type GetNFTSwaggoInput struct {
	Did string `json:"did"`
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
	did := s.GetQuerry(req, "did")
	resp := s.c.GetNFTsByDid(did)
	return s.RenderJSON(req, resp, http.StatusOK)
}

// ShowAccount godoc
// @Summary      Add NFTs
// @Description  This API will put NFTs for sale
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        data      	   body      core.NFTSaleReq  true  "NFT Detials"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/addnftsale [post]
// func (s *Server) APIAddNFTSale(req *ensweb.Request) *ensweb.Result {
// 	did := s.GetQuerry(req, "did")
// 	resp := s.c.GetAllNFT(did)
// 	return s.RenderJSON(req, resp, http.StatusOK)
// }

type ExecuteNFTSwaggoInput struct {
	NFT        string  `json:"nft"`
	Executor   string  `json:"executor"`
	Receiver   string  `json:"receiver"`
	QuorumType int     `json:"quorum_type"`
	Comment    string  `json:"comment"`
	NFTValue   float64 `json:"nft_value"`
	NFTData    string  `json:"nft_data"`
}

// NFT godoc
// @Summary      Execution of NFT
// @Description  This API will add a new block which indicates either transfer of ownership of NFT or internal state change through self-execution
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param		 input body ExecuteNFTSwaggoInput true "Transfer the ownership of particular NFT or self-execution with some data if 'receiver' is empty "
// @Success      200  {object}  model.BasicResponse
// @Router       /api/execute-nft [post]
func (s *Server) APIExecuteNFT(req *ensweb.Request) *ensweb.Result {
	var executeReq model.ExecuteNFTRequest
	err := s.ParseJSON(req, &executeReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", err)
	}
	_, did, ok := util.ParseAddress(executeReq.Executor)
	if !ok {
		return s.BasicResponse(req, false, "Invalid Owner address", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(executeReq.NFT)
	if len(executeReq.NFT) != 46 || !strings.HasPrefix(executeReq.NFT, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid NFT")
		return s.BasicResponse(req, false, "Invalid NFT", nil)
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	s.log.Info("The did trying to transfer the nft :", executeReq.Executor)
	if !strings.HasPrefix(executeReq.Executor, "bafybmi") || len(executeReq.Executor) != 59 || !is_alphanumeric {
		s.log.Error("Invalid owner DID")
		return s.BasicResponse(req, false, "Invalid owner DID", nil)
	}
	if executeReq.QuorumType < 1 || executeReq.QuorumType > 2 {
		s.log.Error("Invalid quorum type")
		return s.BasicResponse(req, false, "Invalid quorum type", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.ExecuteNFT(req.ID, &executeReq)
	return s.didResponse(req, req.ID)
}

type NewNFTSwaggoInput struct {
	NFT string `json:"nft"`
}

// NFT godoc
// @Summary      Subscribe to NFT
// @Description  This API endpoint allows subscribing to a NFT.
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param        input body NewNFTSwaggoInput true "Subscribe to input nft"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/subscribe-nft [post]
func (s *Server) APISubscribeNFT(request *ensweb.Request) *ensweb.Result {
	var newSubscription model.NewNFTSubscription
	err := s.ParseJSON(request, &newSubscription)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(newSubscription.NFT)
	if len(newSubscription.NFT) != 46 || !strings.HasPrefix(newSubscription.NFT, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid NFT")
		return s.BasicResponse(request, false, "Invalid NFT", nil)
	}
	topic := newSubscription.NFT
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
	fetchNft.NFT = s.GetQuerry(req, "nft")

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
