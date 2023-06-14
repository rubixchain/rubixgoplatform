package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core"
)

type InitSmartContractToken struct {
	binaryCodeHash string
	rawCodeHash    string
	yamlCodeHash   string
	genesisBlock   string
}

// DeplotSmartContract godoc
// @Summary      Deploy Smart Contract
// @Description  This API will deploy smart contract
// @Tags         Smart Contract
// @Accept       mpfd
// @Produce      mpfd
// @Param        did        	   formData      string  true   "DID"
// @Param 		 binaryCodePath	   formData      file    true  "location of binary code hash"
// @Param 		 rawCodePath	   formData      file    true  "location of raw code hash"
// @Param 		 yamlFilePath	   formData      file    true  "location of yaml code hash"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/generate-smart-contract [post]
func (s *Server) APIGenerateSmartContract(req *ensweb.Request) *ensweb.Result {
	var deploySC core.GenerateSmartContractRequest
	var err error
	deploySC.SCPath, err = s.c.CreateSCTempFolder()
	if err != nil {
		s.log.Error("Generate smart contract failed, failed to create SC folder", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to create SC folder", nil)
	}

	s.log.Debug("Smart contract folder created", "folder", deploySC.SCPath)

	binaryCodeFile, binaryHeader, err := s.ParseMultiPartFormFile(req, "binaryCodePath")
	if err != nil {
		s.log.Error("Generate smart contract failed, failed to retrieve Binary File", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to retrieve Binary File", nil)
	}

	binaryCodeDest := filepath.Join(deploySC.SCPath, binaryHeader.Filename)
	binaryCodeDestFile, err := os.Create(binaryCodeDest)
	if err != nil {
		binaryCodeFile.Close()
		s.log.Error("Generate smart contract failed, failed to create Binary Code file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to create Binary Code file", nil)
	}

	s.log.Debug("Binary code file retrieved", "filename", binaryHeader.Filename)

	err = os.Rename(binaryCodeFile.Name(), binaryCodeDest)
	if err != nil {
		binaryCodeFile.Close()
		s.log.Error("Generate smart contract failed, failed to move binary code file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to move binary code file", nil)
	}

	rawCodeFile, rawHeader, err := s.ParseMultiPartFormFile(req, "rawCodePath")
	if err != nil {
		binaryCodeDestFile.Close()
		s.log.Error("Generate smart contract failed, failed to retrieve Raw Code file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to retrieve Raw Code file", nil)
	}

	rawCodeDest := filepath.Join(deploySC.SCPath, rawHeader.Filename)
	rawCodeDestFile, err := os.Create(rawCodeDest)
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeFile.Close()
		s.log.Error("Generate smart contract failed, failed to create Raw Code file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to create Raw Code file", nil)
	}

	err = os.Rename(rawCodeFile.Name(), rawCodeDest)
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		s.log.Error("Generate smart contract failed, failed to move raw code file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to move raw code file", nil)
	}

	yamlFile, yamlHeader, err := s.ParseMultiPartFormFile(req, "yamlFilePath")
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		s.log.Error("Generate smart contract failed, failed to retrieve YAML file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to retrieve YAML file", nil)
	}

	yamlDest := filepath.Join(deploySC.SCPath, yamlHeader.Filename)
	yamlDestFile, err := os.Create(yamlDest)
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		yamlFile.Close()
		s.log.Error("Generate smart contract failed, failed to create YAML file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to create YAML file", nil)
	}

	err = os.Rename(yamlFile.Name(), yamlDest)
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		yamlDestFile.Close()
		s.log.Error("Generate smart contract failed, failed to move YAML file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to move YAML file", nil)
	}

	// Close all files
	binaryCodeDestFile.Close()
	rawCodeDestFile.Close()
	yamlDestFile.Close()
	binaryCodeFile.Close()
	rawCodeFile.Close()
	yamlFile.Close()

	deploySC.BinaryCode = binaryCodeDest
	deploySC.RawCode = rawCodeDest
	deploySC.YamlCode = yamlDest

	_, did, err := s.ParseMultiPartForm(req, "did")
	if err != nil {
		s.log.Error("Generate smart contract failed, failed to retrieve DID", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to retrieve DID", nil)
	}

	deploySC.DID = did["did"][0]

	if !s.validateDIDAccess(req, deploySC.DID) {
		return s.BasicResponse(req, false, "Ensure you enter the correct DID", nil)
	}

	s.c.AddWebReq(req)
	go func() {
		basicResponse := s.c.GenerateSmartContractToken(req.ID, &deploySC)
		fmt.Printf("Basic Response server:  %+v\n", *basicResponse)
	}()

	return s.BasicResponse(req, true, "Smart contract generated successfully", nil)
}

// FetchSmartContract godoc
// @Summary      Deploy Smart Contract
// @Description  This API will deploy smart contract
// @Tags         Smart Contract
// @Accept       mpfd
// @Produce      mpfd
// @Param        smartContractToken        	   formData      string  true   "smartContractToken"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/fetch-smart-contract [post]
func (s *Server) APIFetchSmartContract(req *ensweb.Request) *ensweb.Result {
	var fetchSC core.FetchSmartContractRequest
	var err error
	fetchSC.SmartContractTokenPath, err = s.c.CreateSCTempFolder()
	if err != nil {
		s.log.Error("Fetch smart contract failed, failed to create smartcontract folder", "err", err)
		return s.BasicResponse(req, false, "Fetch smart contract failed, failed to create smartcontract folder", nil)
	}

	_, scToken, err := s.ParseMultiPartForm(req, "smartContractToken")
	fetchSC.SmartContractToken = scToken["smartContractToken"][0]
	if err != nil {
		s.log.Error("Fetch smart contract failed, failed to fetch smartcontract token value", "err", err)
		return s.BasicResponse(req, false, "Fetch smart contract failed, failed to fetch smartcontract token value", nil)
	}
	fetchSC.SmartContractTokenPath, err = s.c.RenameSCFolder(fetchSC.SmartContractTokenPath, fetchSC.SmartContractToken)
	if err != nil {
		s.log.Error("Fetch smart contract failed, failed to create SC folder", "err", err)
		return s.BasicResponse(req, false, "Fetch smart contract failed, failed to create SC folder", nil)
	}

	fmt.Printf("fetchSC : %+v\n", fetchSC)

	s.c.AddWebReq(req)
	go func() {
		basicResponse := s.c.FetchSmartContract(req.ID, &fetchSC)
		fmt.Printf("Basic Response server:  %+v\n", *basicResponse)
	}()
	return s.BasicResponse(req, true, "Smart contract fetched successfully", nil)

}
