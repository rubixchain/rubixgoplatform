package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type SmartContractRequest struct {
	BinaryCode string
	RawCode    string
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

func (c *Client) FetchSmartContract(fetchSmartContractRequest *FetchSmartContractRequest) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	if fetchSmartContractRequest.SmartContractToken != "" {
		fields["smartContractToken"] = fetchSmartContractRequest.SmartContractToken
	}

	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIFetchSmartContract, fields, nil, &basicResponse)
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

// List all smart contracts
// GET /rubix/v1/smart_contracts
func (c *Client) ListSmartContracts() (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIListSmartContracts, nil, nil, &basicResponse)
	if err != nil {
		c.log.Error("Failed to list smart contracts", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}

// Get smart contract chain by contract ID
// GET /rubix/v1/smart_contracts/{contract_id}/chain
func (c *Client) GetSmartContractChain(contractID string) (*model.BasicResponse, error) {
	endpoint, err := ensweb.SubstitutePathParams(setup.APIGetSmartContractChain, map[string]string{"contract_id": contractID})
	if err != nil {
		c.log.Error("Failed to construct endpoint for GetSmartContractChain", "err", err)
		return nil, err
	}

	var basicResponse model.BasicResponse
	err = c.sendJSONRequest("GET", endpoint, nil, nil, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to get smart contract chain", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}
