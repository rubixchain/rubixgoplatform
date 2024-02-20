package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ShowAccount godoc
// @Summary      Mint NFT
// @Description  This API will Mint new NFT
// @Tags         NFT
// @Accept       mpfd
// @Produce      mpfd
// @Param        did formData string  true  "DID"
// @Param        digitalAssetPath formData file  true  "Location of the digital Asset"
// @Param        digitalAssetAttributeFilePath formData file true  "Location of the Attribute.json file of the digital asset"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/mintnft [post]
func (s *Server) APIMintNFT(req *ensweb.Request) *ensweb.Result {
	var mintNFT core.MintNFTRequest
	var err error
	mintNFT.NFTPath, err = s.c.CreateFolder("NFT") //create temp NFT folder
	if err != nil {
		s.log.Error("Mint NFT failed, failed to create nft folder", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to create NFT folder", nil)
	}

	//read and upload the digital asset file
	digitalAssetFile, digitalAssetHeader, err := s.ParseMultiPartFormFile(req, "digitalAssetPath")
	if err != nil {
		s.log.Error("Mint NFT failed, failed to retrieve Digital Asset File", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to retrieve Digital Asset File", nil)
	}

	digitalAssetDest := filepath.Join(mintNFT.NFTPath, digitalAssetHeader.Filename)
	digitalAssetDestFile, err := os.Create(digitalAssetDest)
	if err != nil {
		digitalAssetFile.Close()
		s.log.Error("Mint NFT failed, failed to create Digital Asset file", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to create Digital Asset file", nil)
	}
	digitalAssetFile.Close()
	digitalAssetDestFile.Close()

	err = os.Rename(digitalAssetFile.Name(), digitalAssetDest)
	if err != nil {
		digitalAssetFile.Close()
		s.log.Error("Mint NFT failed, failed to move Digital Asset file", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to move Digital Asset file", nil)
	}

	//read and upload the digital asset attribute file
	digitalAssetAttributeFile, digitalAssetAttributeHeader, err := s.ParseMultiPartFormFile(req, "digitalAssetAttributeFilePath")
	if err != nil {
		s.log.Error("Mint NFT failed, failed to retrieve Digital Asset Attribute File", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to retrieve Digital Asset Attribute nFile", nil)
	}

	digitalAssetAttributeDest := filepath.Join(mintNFT.NFTPath, digitalAssetAttributeHeader.Filename)
	digitalAssetAttributeDestFile, err := os.Create(digitalAssetAttributeDest)
	if err != nil {
		digitalAssetFile.Close()
		s.log.Error("Mint NFT failed, failed to create Digital Asset Attribute file", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to create Digital Asset Attribute file", nil)
	}

	digitalAssetAttributeFile.Close()
	digitalAssetAttributeDestFile.Close()

	err = os.Rename(digitalAssetAttributeFile.Name(), digitalAssetAttributeDest)
	if err != nil {
		digitalAssetFile.Close()
		s.log.Error("Mint NFT failed, failed to move Digital Asset Attribute file", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to move Digital Asset Attribute file", nil)
	}

	//close all files
	digitalAssetDestFile.Close()
	digitalAssetAttributeDestFile.Close()
	digitalAssetFile.Close()
	digitalAssetAttributeFile.Close()

	//add the paths of the temporary folder where the files have been stored
	mintNFT.DigitalAssetPath = digitalAssetDest
	mintNFT.DigitalAssetAttributeFile = digitalAssetAttributeDest

	_, did, err := s.ParseMultiPartForm(req, "did")
	if err != nil {
		s.log.Error("Mint NFT failed, failed to retrieve DID", "err", err)
		return s.BasicResponse(req, false, "Mint NFT failed, failed to retrieve DID", nil)
	}

	mintNFT.DID = did["did"][0]
	if !s.validateDIDAccess(req, mintNFT.DID) {
		return s.BasicResponse(req, false, "Ensure you enter the correct DID", nil)
	}

	s.c.AddWebReq(req)
	go func() {
		basicResponse := s.c.MintNFT(req.ID, &mintNFT)
		fmt.Printf("Basic Response server:  %+v\n", *basicResponse)
	}()

	return s.BasicResponse(req, true, "NFT minted successfully", nil)
}

// ShowAccount godoc
// @Summary      Get ALL NFTs
// @Description  This API will get all NFTs of the DID
// @Tags         NFT
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.NFTTokens
// @Router       /api/getallnft [post]
func (s *Server) APIGetAllNFT(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	resp := s.c.GetAllNFT(did)
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
func (s *Server) APIAddNFTSale(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	resp := s.c.GetAllNFT(did)
	return s.RenderJSON(req, resp, http.StatusOK)
}
