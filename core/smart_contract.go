package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

type GenerateSmartContractRequest struct {
	BinaryCode string
	RawCode    string
	YamlCode   string
	DID        string
	SCPath     string
}

type SmartContractToken struct {
	BinaryCodeHash string `json:"binaryCodeHash"`
	RawCodeHash    string `json:"rawCodeHash"`
	YamlCodeHash   string `json:"yamlCodeHash"`
	DID            string `json:"did"`
}

type FetchSmartContractRequest struct {
	SmartContractToken     string
	SmartContractTokenPath string
}

type SmartContractTokenResponse struct {
	Message string `json:"message"`
	Result  string `json:"result"`
}

func (c *Core) GenerateSmartContractToken(requestID string, smartContractTokenRequest *GenerateSmartContractRequest) *model.BasicResponse {

	defer os.RemoveAll(smartContractTokenRequest.SCPath)

	smartContractTokenResponse := c.generateSmartContractToken(requestID, smartContractTokenRequest)
	dc := c.GetWebReq(requestID)
	if dc == nil {
		c.log.Error("failed to get web request", "requestID", requestID)
		return nil
	}
	dc.OutChan <- smartContractTokenResponse

	return smartContractTokenResponse
}

func (c *Core) generateSmartContractToken(requestID string, smartContractTokenRequest *GenerateSmartContractRequest) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}

	binaryCodeFile, err := os.Open(smartContractTokenRequest.BinaryCode)
	if err != nil {
		c.log.Error("Failed to open binary code file", "err", err)
		return basicResponse
	}
	defer binaryCodeFile.Close()

	// Add binary code file to IPFS
	binaryCodeHash, err := c.ipfs.Add(binaryCodeFile)
	if err != nil {
		c.log.Error("Failed to add binary code file to IPFS", "err", err)
		return basicResponse
	}

	// Open raw code file
	rawCodeFile, err := os.Open(smartContractTokenRequest.RawCode)
	if err != nil {
		c.log.Error("Failed to open raw code file", "err", err)
		return basicResponse
	}
	defer rawCodeFile.Close()

	// Add raw code file to IPFS
	rawCodeHash, err := c.ipfs.Add(rawCodeFile)
	if err != nil {
		c.log.Error("Failed to add raw code file to IPFS", "err", err)
		return basicResponse
	}

	// Open YAML code file
	yamlCodeFile, err := os.Open(smartContractTokenRequest.YamlCode)
	if err != nil {
		c.log.Error("Failed to open YAML code file", "err", err)
		return basicResponse
	}
	defer yamlCodeFile.Close()

	// Add YAML code file to IPFS
	yamlCodeHash, err := c.ipfs.Add(yamlCodeFile)
	if err != nil {
		c.log.Error("Failed to add YAML code file to IPFS", "err", err)
		return basicResponse
	}

	smartContractToken := SmartContractToken{
		BinaryCodeHash: binaryCodeHash,
		RawCodeHash:    rawCodeHash,
		YamlCodeHash:   yamlCodeHash,
		DID:            smartContractTokenRequest.DID,
	}

	if err != nil {
		c.log.Error("Failed to create smart contract token", "err", err)
		return basicResponse
	}

	smartContractTokenJSON, err := json.MarshalIndent(smartContractToken, "", "  ")
	if err != nil {
		c.log.Error("Failed to marshal SmartContractToken struct", "err", err)
		return basicResponse
	}

	smartContractTokenHash, err := c.ipfs.Add(bytes.NewReader(smartContractTokenJSON))
	if err != nil {
		c.log.Error("Failed to add SmartContractToken to IPFS", "err", err)
		return basicResponse
	}

	fmt.Println("smartContractTokenHash ", smartContractTokenHash)

	// Set the response status and message
	smartContractTokenResponse := &SmartContractTokenResponse{
		Message: "Smart contract generated successfully",
		Result:  smartContractTokenHash,
	}

	_, err = c.RenameSCFolder(smartContractTokenRequest.SCPath, smartContractTokenHash)
	if err != nil {
		c.log.Error("Failed to rename SC folder", "err", err)
		return basicResponse
	}
	err = c.w.CreateSmartContractToken(&wallet.SmartContract{SmartContractHash: smartContractTokenHash, Deployer: smartContractTokenRequest.DID, BinaryCodeHash: binaryCodeHash, RawCodeHash: rawCodeHash, YamlCodeHash: yamlCodeHash, ContractStatus: 6})

	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = smartContractTokenResponse.Message
	basicResponse.Result = smartContractTokenResponse

	return basicResponse
}

func (c *Core) FetchSmartContract(requestID string, fetchSmartContractRequest *FetchSmartContractRequest) *model.BasicResponse {

	basicResponse := &model.BasicResponse{
		Status: false,
	}

	smartContractTokenJSON, err := c.ipfs.Cat(fetchSmartContractRequest.SmartContractToken)
	if err != nil {
		c.log.Error("Failed to get smart contract from network", "err", err)
		return basicResponse
	}

	// Read the smart contract token JSON
	smartContractTokenJSONBytes, err := ioutil.ReadAll(smartContractTokenJSON)
	if err != nil {
		c.log.Error("Failed to read smart contract token from network", "err", err)
		return basicResponse
	}

	// Close the smart contract token JSON reader
	smartContractTokenJSON.Close()

	// Parse smart contract token JSON into SmartContractToken struct
	var smartContractToken SmartContractToken
	err = json.Unmarshal(smartContractTokenJSONBytes, &smartContractToken)
	if err != nil {
		c.log.Error("Failed to parse smart contract token", "err", err)
		return basicResponse
	}

	// Fetch and store the binary code file
	binaryCodeFile, err := c.ipfs.Cat(smartContractToken.BinaryCodeHash)
	if err != nil {
		c.log.Error("Failed to fetch binary code file from network", "err", err)
		return basicResponse
	}
	defer binaryCodeFile.Close()

	binaryCodeFilePath := fetchSmartContractRequest.SmartContractTokenPath
	err = os.MkdirAll(binaryCodeFilePath, 0755)
	if err != nil {
		c.log.Error("Failed to create binary code directory", "err", err)
		return basicResponse
	}

	binaryCodeFileDestPath := filepath.Join(binaryCodeFilePath, "binaryCodeFile")

	// Read the content of binaryCodeFile
	binaryCodeContent, err := ioutil.ReadAll(binaryCodeFile)
	if err != nil {
		c.log.Error("Failed to read binary code file", "err", err)
		return basicResponse
	}

	// Write the content to binaryCodeFileDestPath
	err = ioutil.WriteFile(binaryCodeFileDestPath, binaryCodeContent, 0644)
	if err != nil {
		c.log.Error("Failed to write binary code file", "err", err)
		return basicResponse
	}

	// Fetch and store the raw code file
	rawCodeFile, err := c.ipfs.Cat(smartContractToken.RawCodeHash)
	if err != nil {
		c.log.Error("Failed to fetch raw code file from IPFS", "err", err)
		return basicResponse
	}
	defer rawCodeFile.Close()

	rawCodeFilePath := fetchSmartContractRequest.SmartContractTokenPath
	err = os.MkdirAll(rawCodeFilePath, 0755)
	if err != nil {
		c.log.Error("Failed to create raw code directory", "err", err)
		return basicResponse
	}

	rawCodeFileDestPath := filepath.Join(rawCodeFilePath, "rawCodeFile")

	// Read the content of rawCodeFile
	rawCodeContent, err := ioutil.ReadAll(rawCodeFile)
	if err != nil {
		c.log.Error("Failed to read raw code file", "err", err)
		return basicResponse
	}

	// Write the content to rawCodeFileDestPath
	err = ioutil.WriteFile(rawCodeFileDestPath, rawCodeContent, 0644)
	if err != nil {
		c.log.Error("Failed to write raw code file", "err", err)
		return basicResponse
	}

	// Fetch and store the YAML code file
	yamlCodeFile, err := c.ipfs.Cat(smartContractToken.YamlCodeHash)
	if err != nil {
		c.log.Error("Failed to fetch YAML code file from IPFS", "err", err)
		return basicResponse
	}
	defer yamlCodeFile.Close()

	yamlCodeFilePath := fetchSmartContractRequest.SmartContractTokenPath
	err = os.MkdirAll(yamlCodeFilePath, 0755)
	if err != nil {
		c.log.Error("Failed to create YAML directory", "err", err)
		return basicResponse
	}

	yamlCodeFileDestPath := filepath.Join(yamlCodeFilePath, "yamlCodeFile")

	// Read the content of yamlCodeFile
	yamlCodeContent, err := ioutil.ReadAll(yamlCodeFile)
	if err != nil {
		c.log.Error("Failed to read YAML code file", "err", err)
		return basicResponse
	}

	// Write the content to yamlCodeFileDestPath
	err = ioutil.WriteFile(yamlCodeFileDestPath, yamlCodeContent, 0644)
	if err != nil {
		c.log.Error("Failed to write YAML code file", "err", err)
		return basicResponse
	}

	err = c.w.CreateSmartContractToken(&wallet.SmartContract{SmartContractHash: fetchSmartContractRequest.SmartContractToken, Deployer: smartContractToken.DID, BinaryCodeHash: smartContractToken.BinaryCodeHash, RawCodeHash: smartContractToken.RawCodeHash, YamlCodeHash: smartContractToken.YamlCodeHash, ContractStatus: wallet.TokenIsFetched})

	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = "Successfully fetched smart contract"
	basicResponse.Result = &smartContractToken

	return basicResponse
}
