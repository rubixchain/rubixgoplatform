package client

import (

	"time"
  "fmt"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)


type SmartContractRequest struct {
	BinaryCode string
	RawCode    string
	SchemaCode string
	DID        string
	SCPath     string
}

type FetchSmartContractRequest struct {
	SmartContractToken     string
	SmartContractTokenPath string
}

func (c *Client) DeploySmartContract(deployRequest *model.DeploySmartContractRequest) (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIDeploySmartContract, nil, deployRequest, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Deploy Smart Contract", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}

func (c *Client) GenerateSmartContractToken(smartContractRequest *SmartContractRequest) (*model.BasicResponse, error) {

	fields := make(map[string]string)
	files := make(map[string]string)

	if smartContractRequest.BinaryCode != "" {
		files["binaryCodePath"] = smartContractRequest.BinaryCode
	}
	if smartContractRequest.RawCode != "" {
		files["rawCodePath"] = smartContractRequest.RawCode
	}
	if smartContractRequest.SchemaCode != "" {
		files["schemaFilePath"] = smartContractRequest.SchemaCode
	}
	if smartContractRequest.DID != "" {
		fields["did"] = smartContractRequest.DID
	}

	for key, value := range fields {
		fmt.Printf("Field: %s, Value: %s\n", key, value)
	}

	for key, value := range files {
		fmt.Printf("File: %s, Value: %s\n", key, value)
	}

	var basicResponse model.BasicResponse
	err := c.sendMutiFormRequest("POST", "/api/generate-smart-contract", nil, fields, files, &basicResponse)
	if err != nil {
		return nil, err
	}

	return &basicResponse, nil

}

func (c *Client) FetchSmartContract(fetchSmartContractRequest *FetchSmartContractRequest) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	if fetchSmartContractRequest.SmartContractToken != "" {
		fields["smartContractToken"] = fetchSmartContractRequest.SmartContractToken
	}

	var basicResponse model.BasicResponse
	err := c.sendMutiFormRequest("POST", "/api/fetch-smart-contract", nil, fields, nil, &basicResponse)
	if err != nil {
		return nil, err
	}
	return &basicResponse, nil

}
