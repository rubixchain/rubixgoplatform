package server

import (
	"net/http"
	"strconv"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ShowAccount godoc
// @Summary      Create NFT
// @Description  This API will create new NFT
// @Tags         NFT
// @Accept       mpfd
// @Produce      mpfd
// @Param        UserInfo      	   formData      string  false  "User/Entity Info"
// @Param        FileInfo    	   formData      string  false  "File Info is json string {"FileHash" : "asja", "FileURL": "ass", "FileTransInfo": "asas"}"
// @Param        FileContent       formData      file    false  "File to be committed"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/createnft [post]
func (s *Server) APICreateNFT(req *ensweb.Request) *ensweb.Result {
	var nr core.NFTReq
	var err error
	nr.DID = s.GetQuerry(req, "did")
	nr.NumTokens = 1
	numTokens := s.GetQuerry(req, "numTokens")
	if numTokens != "" {
		nt, err := strconv.ParseInt(numTokens, 10, 32)
		if err == nil {
			nr.NumTokens = int(nt)
		}
	}
	nr.FolderName, err = s.c.CreateTempFolder()
	if err != nil {
		s.log.Error("failed to create folder", "err", err)
		return s.BasicResponse(req, false, "failed to create folder", nil)
	}
	nr.FileNames, nr.Fields, err = s.ParseMultiPartForm(req, nr.FolderName+"/")

	if err != nil {
		s.log.Error("Create data token failed, failed to parse request", "err", err)
		return s.BasicResponse(req, false, "Create data token failed, failed to parse request", nil)
	}

	if !s.validateDIDAccess(req, nr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.CreateNFT(req.ID, &nr)
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
