package client

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
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
	err := c.sendJSONRequest("POST", setup.APIDeploySmartContract, nil, deployRequest, &basicResponse, time.Minute*2)
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
	err := c.sendMutiFormRequest("POST", setup.APIGenerateSmartContract, nil, fields, files, &basicResponse)
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
	err := c.sendMutiFormRequest("POST", setup.APIFetchSmartContract, nil, fields, nil, &basicResponse)
	if err != nil {
		return nil, err
	}
	return &basicResponse, nil

}

func (c *Client) PublishNewEvent(smartContractToken string, did string, publishType int, block string) (*model.BasicResponse, error) {
	var response model.BasicResponse
	newContract := model.NewContractEvent{
		SmartContractToken:     smartContractToken,
		Did:                    did,
		Type:                   publishType,
		SmartContractBlockHash: block,
	}
	err := c.sendJSONRequest("POST", setup.APIPublishContract, nil, &newContract, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
func (c *Client) SubscribeContract(smartContractToken string) (*model.BasicResponse, error) {
	var response model.BasicResponse
	newSubscription := model.NewSubscription{
		SmartContractToken: smartContractToken,
	}
	err := c.sendJSONRequest("POST", setup.APISubscribecontract, nil, &newSubscription, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ExecuteSmartContract(executeRequest *model.ExecuteSmartContractRequest) (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIExecuteSmartContract, nil, executeRequest, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Execute Smart Contract", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}
