package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

type InitSmartContractToken struct {
	binaryCodeHash string
	rawCodeHash    string
	schemaCodeHash string
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
// @Param 		 schemaFilePath	   formData      file    true  "location of schema code hash"
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

	binaryCodeFile.Close()
	binaryCodeDestFile.Close()

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

	rawCodeFile.Close()
	rawCodeDestFile.Close()

	err = os.Rename(rawCodeFile.Name(), rawCodeDest)
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		s.log.Error("Generate smart contract failed, failed to move raw code file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to move raw code file", nil)
	}

	schemaFile, schemaHeader, err := s.ParseMultiPartFormFile(req, "schemaFilePath")
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		s.log.Error("Generate smart contract failed, failed to retrieve Schema file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to retrieve Schema file", nil)
	}

	schemaDest := filepath.Join(deploySC.SCPath, schemaHeader.Filename)
	schemaDestFile, err := os.Create(schemaDest)
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		schemaFile.Close()
		s.log.Error("Generate smart contract failed, failed to create Schema file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to create Schema file", nil)
	}

	schemaFile.Close()
	schemaDestFile.Close()

	err = os.Rename(schemaFile.Name(), schemaDest)
	if err != nil {
		binaryCodeDestFile.Close()
		rawCodeDestFile.Close()
		schemaDestFile.Close()
		s.log.Error("Generate smart contract failed, failed to move Schema file", "err", err)
		return s.BasicResponse(req, false, "Generate smart contract failed, failed to move Schema file", nil)
	}

	// Close all files
	binaryCodeDestFile.Close()
	rawCodeDestFile.Close()
	schemaDestFile.Close()
	binaryCodeFile.Close()
	rawCodeFile.Close()
	schemaFile.Close()

	deploySC.BinaryCode = binaryCodeDest
	deploySC.RawCode = rawCodeDest
	deploySC.SchemaCode = schemaDest

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
func (s *Server) APIPublishContract(request *ensweb.Request) *ensweb.Result {
	var newEvent model.NewContractEvent
	err := s.ParseJSON(request, &newEvent)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}

	go s.c.PublishNewEvent(&newEvent)
	return s.BasicResponse(request, true, "Smart contract published successfully", nil)
}
func (s *Server) APISubscribecontract(request *ensweb.Request) *ensweb.Result {
	var newSubscription model.NewSubcription
	err := s.ParseJSON(request, &newSubscription)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}
	topic := newSubscription.Contract
	s.c.AddWebReq(request)
	go s.c.SubsribeContractSetup(request.ID, topic)
	return s.BasicResponse(request, true, "Smart contract subscribed successfully", nil)
}
