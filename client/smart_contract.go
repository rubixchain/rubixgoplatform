package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

type SmartContractRequest struct {
	BinaryCode string
	RawCode    string
	DID        string
	SCPath     string
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
	if smartContractRequest.DID != "" {
		fields["did"] = smartContractRequest.DID
	}
	var basicResponse model.BasicResponse
	err := c.sendMutiFormRequest("POST", setup.APIGenerateSmartContract, nil, fields, files, &basicResponse)
	if err != nil {
		return nil, err
	}
	return &basicResponse, nil

}

func (c *Client) FetchSmartContract(smartContractToken string) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	if smartContractToken != "" {
		fields["smartContractToken"] = smartContractToken
	}

	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIFetchSmartContract, fields, nil, &basicResponse)
	if err != nil {
		return nil, err
	}
	return &basicResponse, nil
}

func (c *Client) SubscribeContract(smartContractToken string) (*model.BasicResponse, error) {
	var response model.BasicResponse
	// Use query parameter instead of JSON body
	query := make(map[string]string)
	query["smartContractToken"] = smartContractToken
	err := c.sendJSONRequest("POST", setup.APISubscribecontract, query, nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
