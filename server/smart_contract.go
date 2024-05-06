package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type InitSmartContractToken struct {
	binaryCodeHash string
	rawCodeHash    string
	schemaCodeHash string
	genesisBlock   string
}

type FetchSmartContractSwaggoInput struct {
	SmartContractToken string `json:"smartContractToken"`
}

type NewSubscriptionSwaggoInput struct {
	SmartContractToken string `json:"smartContractToken"`
}

type DeploySmartContractSwaggoInput struct {
	SmartContractToken string  `json:"smartContractToken"`
	DeployerAddress    string  `json:"deployerAddr"`
	RBTAmount          float64 `json:"rbtAmount"`
	QuorumType         int     `json:"quorumType"`
	Comment            string  `json:"comment"`
}

// SmartContract godoc
// @Summary      Deploy Smart Contract
// @Description  This API will deploy smart contract Token
// @Tags         Smart Contract
// @ID 			 deploy-smart-contract
// @Accept       json
// @Produce      json
// @Param		 input body DeploySmartContractSwaggoInput true "Deploy smart contract"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/deploy-smart-contract [post]
func (s *Server) APIDeploySmartContract(req *ensweb.Request) *ensweb.Result {
	var deployReq model.DeploySmartContractRequest
	err := s.ParseJSON(req, &deployReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(deployReq.DeployerAddress)
	if !ok {
		return s.BasicResponse(req, false, "Invalid Deployer address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.DeploySmartContractToken(req.ID, &deployReq)
	return s.didResponse(req, req.ID)
}

// SmartContract godoc
// @Summary      Generate Smart Contract
// @Description  This API will Generate smart contract Token
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

	err = moveFile(binaryCodeFile.Name(), binaryCodeDest)
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

	err = moveFile(rawCodeFile.Name(), rawCodeDest)
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

	err = moveFile(schemaFile.Name(), schemaDest)
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

// moveFile tries to rename the file first; if it fails, it falls back to copying
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err != nil {
		if linkErr, ok := err.(*os.LinkError); ok {
			fmt.Println("os.Rename failed, attempting to copy:", linkErr)

			// Open the source file
			sourceFile, err := os.Open(src)
			if err != nil {
				return fmt.Errorf("error opening source file: %w", err)
			}
			defer sourceFile.Close()

			// Create the destination file
			destinationFile, err := os.Create(dst)
			if err != nil {
				return fmt.Errorf("error creating destination file: %w", err)
			}
			defer destinationFile.Close()

			// Copy the contents
			if _, err = io.Copy(destinationFile, sourceFile); err != nil {
				return fmt.Errorf("error copying file: %w", err)
			}

			// Close the files explicitly before deleting
			if err = sourceFile.Close(); err != nil {
				return fmt.Errorf("error closing source file: %w", err)
			}
			if err = destinationFile.Close(); err != nil {
				return fmt.Errorf("error closing destination file: %w", err)
			}

			// Delete the original file
			if err = os.Remove(src); err != nil {
				return fmt.Errorf("error removing original file: %w", err)
			}
		} else {
			return fmt.Errorf("os.Rename error: %w", err)
		}
	}
	return nil
}

// SmartContract godoc
// @Summary      Fetch Smart Contract
// @Description  This API will Fetch smart contract
// @Tags         Smart Contract
// @ID   	     fetch-smart-contract
// @Accept       json
// @Produce      json
// @Param        input body FetchSmartContractSwaggoInput true "Fetch smart contract"
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

// SmartContract godoc
// @Summary      Subscribe to Smart Contract
// @Description  This API endpoint allows subscribing to a smart contract.
// @Tags         Smart Contract
// @Accept       json
// @Produce      json
// @Param        input body NewSubscriptionSwaggoInput true "Subscribe to input contract"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/subscribe-smart-contract [post]
func (s *Server) APISubscribecontract(request *ensweb.Request) *ensweb.Result {
	var newSubscription model.NewSubscription
	err := s.ParseJSON(request, &newSubscription)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}
	topic := newSubscription.SmartContractToken
	s.c.AddWebReq(request)
	go s.c.SubsribeContractSetup(request.ID, topic)
	return s.BasicResponse(request, true, "Smart contract subscribed successfully", nil)
}

type ExecuteSmartContractSwaggoInput struct {
	SmartContractToken string `json:"smartContractToken"`
	ExecutorAddress    string `json:"executorAddr"`
	QuorumType         int    `json:"quorumType"`
	Comment            string `json:"comment"`
	SmartContractData  string `json:"smartContractData"`
}

// SmartContract godoc
// @Summary      Execute Smart Contract
// @Description  This API will Execute smart contract Token
// @Tags         Smart Contract
// @Accept       json
// @Produce      json
// @Param		 input body ExecuteSmartContractSwaggoInput true "Execute smart contrct and add details to chain"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/execute-smart-contract [post]
func (s *Server) APIExecuteSmartContract(req *ensweb.Request) *ensweb.Result {
	var executeReq model.ExecuteSmartContractRequest
	err := s.ParseJSON(req, &executeReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(executeReq.ExecutorAddress)
	if !ok {
		return s.BasicResponse(req, false, "Invalid Deployer address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.ExecuteSmartContractToken(req.ID, &executeReq)
	return s.didResponse(req, req.ID)
}
