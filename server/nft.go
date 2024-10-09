package server

import (
	"fmt"
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
// @Param        UserID      	   formData      string  true  "User/Entity Info"
// @Param        NFTFileInfo       formData      file  true  "NFTFileInfo is a metadata about the file being given. We are expecting a json file with a mandatory key filename"
// @Param        NFTFile       formData      file    true  "File to be committed"
// @Param        Data      	   formData      string  true  "The data which the user wishes to be put in"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/createnft [post]
func (s *Server) APICreateNFT(req *ensweb.Request) *ensweb.Result {
	var createNFT core.NFTReq
	var err error
	createNFT.NFTPath, err = s.c.CreateNFTTempFolder()
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to create NFT folder", "err", err)
		return s.BasicResponse(req, false, "Failed to create NFT, Failed to create NFT folder", nil)
	}
	nftInfoFile, nftInfoFileHeader, err := s.ParseMultiPartFormFile(req, "NFTFileInfo")
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to retrieve NFT file Info", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to retrieve NFT file Info", nil)
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

	nftFile, nftFileHeader, err := s.ParseMultiPartFormFile(req, "NFTFile")
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to retrieve NFT file", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to retrieve NFT file", nil)
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

	createNFT.NFTFile = nftFileDest
	createNFT.NFTFileInfo = nftFileInfoDest

	_, did, err := s.ParseMultiPartForm(req, "did")
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to retrieve DID", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, failed to retrieve DID", nil)
	}
	createNFT.DID = did["did"][0]
	_, userId, err := s.ParseMultiPartForm(req, "UserID")
	if err != nil {
		s.log.Error("Creation of NFT failed, failed to retrieve UserID", "err", err)
		return s.BasicResponse(req, false, "Creation of NFT failed, fialed to retrieve UserID", nil)
	}
	createNFT.UserID = userId["UserID"][0]
	fmt.Println("The userID is ", createNFT.UserID)
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
	NFT        string `json:"NFT"`
	DID        string `json:"DID"`
	QuorumType int    `json:"QuorumType"`
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
// @Description  This API will get all NFTs of the DID
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.NFTTokens
// @Router       /api/getallnft [post]
// func (s *Server) APIGetAllNFT(req *ensweb.Request) *ensweb.Result {
// 	did := s.GetQuerry(req, "did")
// 	resp := s.c.GetAllNFT(did)
// 	return s.RenderJSON(req, resp, http.StatusOK)
// }

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
	NFT        string  `json:"NFT"`
	Executor   string  `json:"Executor"`
	Receiver   string  `json:"Receiver"`
	QuorumType int     `json:"QuorumType"`
	Comment    string  `json:"Comment"`
	NFTValue   float64 `json:"NFTValue"`
	NFTData    string  `json:"NFTData"`
}

// NFT godoc
// @Summary      Execute NFT or transfer of NFT ownership
// @Description  This API will add a new block which indicates the transfer of ownership of NFT
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Param		 input body ExecuteNFTSwaggoInput true "Execute NFT and transfer the ownership of particular NFT"
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
		return s.BasicResponse(req, false, "Invalid Executer address", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(executeReq.NFT)
	if len(executeReq.NFT) != 46 || !strings.HasPrefix(executeReq.NFT, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid NFT")
		return s.BasicResponse(req, false, "Invalid NFT", nil)
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(executeReq.Executor, "bafybmi") || len(executeReq.Executor) != 59 || !is_alphanumeric {
		s.log.Error("Invalid executer DID")
		return s.BasicResponse(req, false, "Invalid executer DID", nil)
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
	go s.c.SubsribeNFTSetup(request.ID, topic)
	return s.BasicResponse(request, true, "NFT subscribed successfully", nil)
}
