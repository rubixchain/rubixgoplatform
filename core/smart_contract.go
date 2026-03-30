package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

const (
	NewStateEvent string = "new_state_event"
)

const (
	DeployType  int = 1
	ExecuteType int = 2
)

type NewState struct {
	ConOwnerDID  string `json:"contract_ownwer_did"`
	ConHash      string `json:"contract_hash"`
	ConBlockHash string `json:"contract_block_hash"`
}

var reqID string

type GenerateSmartContractRequest struct {
	BinaryCode string
	RawCode    string
	DID        string
	SCPath     string
}

type SmartContractToken struct {
	BinaryCodeHash string `json:"binaryCodeHash"`
	RawCodeHash    string `json:"rawCodeHash"`
	DID            string `json:"did"`
	PeerID         string `json:"peerID"`
}

type FetchSmartContractRequest struct {
	SmartContractToken     string
	SmartContractTokenPath string
}

type SmartContractTokenResponse struct {
	Message string `json:"message"`
	Result  string `json:"result"`
}

func (c *Core) GenerateSmartContractToken(requestID string, smartContractTokenRequest *GenerateSmartContractRequest) {

	defer os.RemoveAll(smartContractTokenRequest.SCPath)

	smartContractTokenResponse := c.generateSmartContractToken(requestID, smartContractTokenRequest)
	dc := c.GetWebReq(requestID)
	if dc == nil {
		c.log.Error("failed to get web request", "requestID", requestID)
	}
	dc.OutChan <- smartContractTokenResponse

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
	binaryCodeHash, err := IpfsAddWithBackoff(c.ipfs, binaryCodeFile)
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
	rawCodeHash, err := IpfsAddWithBackoff(c.ipfs, rawCodeFile)
	if err != nil {
		c.log.Error("Failed to add raw code file to IPFS", "err", err)
		return basicResponse
	}

	smartContractToken := SmartContractToken{
		BinaryCodeHash: binaryCodeHash,
		RawCodeHash:    rawCodeHash,
		DID:            smartContractTokenRequest.DID,
		PeerID:         c.peerID,
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

	smartContractTokenHash, err := IpfsAddWithBackoff(c.ipfs, bytes.NewReader(smartContractTokenJSON))
	if err != nil {
		c.log.Error("Failed to add SmartContractToken to IPFS", "err", err)
		return basicResponse
	}

	c.log.Info("smartContractTokenHash ", smartContractTokenHash)

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
	err = c.w.CreateSmartContractToken(&wallet.SmartContract{SmartContractHash: smartContractTokenHash, Deployer: smartContractTokenRequest.DID, BinaryCodeHash: binaryCodeHash, RawCodeHash: rawCodeHash, ContractStatus: wallet.TokenIsGenerated})
	if err != nil {
		c.log.Error("Failed to create smart contract token", "err", err)
		return basicResponse
	}

	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = smartContractTokenResponse.Message
	basicResponse.Result = smartContractTokenResponse.Result

	return basicResponse
}

func (c *Core) FetchSmartContract(requestID string, fetchSmartContractRequest *FetchSmartContractRequest) *model.BasicResponse {

	basicResponse := &model.BasicResponse{
		Status: false,
	}

	smartContractTokenJSON, err := c.ipfsOps.Cat(fetchSmartContractRequest.SmartContractToken)
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
	binaryCodeFile, err := c.ipfsOps.Cat(smartContractToken.BinaryCodeHash)
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
	err = ioutil.WriteFile(binaryCodeFileDestPath+".wasm", binaryCodeContent, 0644)
	if err != nil {
		c.log.Error("Failed to write binary code file", "err", err)
		return basicResponse
	}

	// Fetch and store the raw code file
	rawCodeFile, err := c.ipfsOps.Cat(smartContractToken.RawCodeHash)
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

	err = c.w.CreateSmartContractToken(&wallet.SmartContract{SmartContractHash: fetchSmartContractRequest.SmartContractToken, Deployer: smartContractToken.DID, BinaryCodeHash: smartContractToken.BinaryCodeHash, RawCodeHash: smartContractToken.RawCodeHash, ContractStatus: wallet.TokenIsFetched})
	if err != nil {
		basicResponse.Message = "unable to add Smart contract record to DB, err: " + err.Error()
		return basicResponse
	}

	if smartContractToken.PeerID != "" {
		smartContractOriginPeer, err := c.getPeer(smartContractToken.PeerID + "." + smartContractToken.DID)
		if err != nil {
			basicResponse.Message = fmt.Sprintf("unable to get the peer for DID: %v, err: %v ", smartContractToken.DID, err)
			return basicResponse
		}

		errSync, _ := c.syncTokenChainFrom(smartContractOriginPeer, "", fetchSmartContractRequest.SmartContractToken, c.TokenType("sc"))
		if errSync != nil {
			basicResponse.Message = fmt.Sprintf("unable to sync token chain for contract: %v, err: %v", fetchSmartContractRequest.SmartContractToken, errSync)
			return basicResponse
		}
	} else {
		c.log.Debug(fmt.Sprintf("Unable to fetch Smart Contract %v deployer's PeerID, skipping token chain sync", fetchSmartContractRequest.SmartContractToken))
	}

	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = "Successfully fetched smart contract"
	basicResponse.Result = smartContractToken

	return basicResponse
}

// updating PublishNewEvent to PublishSmartContractEvent with new structs
func (c *Core) publishNewSmartContractEvent(newSCEvent *models.EventSmartContractPublishInfo) error {
	if c.ps == nil {
		return nil
	}

	topic := newSCEvent.SmartContractID

	if err := c.ps.Publish(topic, newSCEvent); err != nil {
		c.log.Error("Failed to publish smart contract event", "topic", topic, "err", err)
		return err
	}

	c.log.Info("New state published on smart contract", "topic", topic)

	return nil
}

func (c *Core) publishSmartContractEvents(
	request *models.TransactionRequest,
	transactionId string,
	initiatorDID string,
	initiatorSignature string,
	epoch int,
) {

	smartContracts := request.GetAllSmartContracts()

	baseEvent := models.EventSmartContractPublishInfo{
		TransactionID:      transactionId,
		Initiator:          initiatorDID,
		InitiatorSignature: initiatorSignature,
		Epoch:              epoch,
	}

	for _, sc := range smartContracts {

		event := baseEvent
		event.SmartContractID = sc.SmartContractId
		event.SmartContractData = sc.Data

		if err := c.publishNewSmartContractEvent(&event); err != nil {
			c.log.Error("Smart contract event publish failed",
				"smartcontract", sc.SmartContractId,
				"err", err,
			)
		}
	}
}

func (c *Core) SubsribeContractSetup(requestID string, topic string) error {
	reqID = requestID
	err := c.ps.SubscribeTopic(topic, c.ContractCallBack)
	if err != nil {
		c.log.Error("Unable to subscribe smart contract ", topic)
	}
	c.log.Info("Subscribing smart contract " + topic + " is successful")
	return err
}

// ContractCallback updated with the syncTransactionChainFrom function
func (c *Core) ContractCallBack(peerID string, topic string, data []byte) {
	var newEvent model.NewContractEvent
	var fetchSC FetchSmartContractRequest
	requestID := reqID
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to get contract details", "err", err)
	}
	c.log.Info("Update on smart contract " + newEvent.SmartContractToken)
	if newEvent.Type == 1 {
		fetchSC.SmartContractToken = newEvent.SmartContractToken
		fetchSC.SmartContractTokenPath, err = c.CreateSCTempFolder()
		if err != nil {
			c.log.Error("Fetch smart contract failed, failed to create smartcontract folder", "err", err)
			return
		}
		// oldScFolder is set to path of the smartcontract folder
		oldScFolder := c.cfg.NodeConfigDir + "SmartContract/" + fetchSC.SmartContractToken
		var isPathExist bool
		//info is set to FileInfo describing the oldScFolder
		info, err := os.Stat(oldScFolder)
		//If directory doesn't exist, isPathExist is set to false
		if os.IsNotExist(err) {
			isPathExist = false
		} else {
			isPathExist = info.IsDir()

		}
		//If isPathExist true, removing the existing folder which was generated while generating the smartcontract hash
		if isPathExist {
			c.log.Debug("removing the existing folder:", oldScFolder, "to add the new folder")
			os.RemoveAll(oldScFolder)
		}
		fetchSC.SmartContractTokenPath, err = c.RenameSCFolder(fetchSC.SmartContractTokenPath, fetchSC.SmartContractToken)
		if err != nil {
			c.log.Error("Fetch smart contract failed, failed to create SC folder", "err", err)
			return
		}
		c.FetchSmartContract(requestID, &fetchSC)
		c.log.Info("Smart contract " + fetchSC.SmartContractToken + " files fetching succesful")
	}
	smartContractToken := newEvent.SmartContractToken
	scFolderPath := c.cfg.NodeConfigDir + "SmartContract/" + smartContractToken
	if _, err := os.Stat(scFolderPath); os.IsNotExist(err) {
		fetchSC.SmartContractToken = smartContractToken
		fetchSC.SmartContractTokenPath, err = c.CreateSCTempFolder()
		if err != nil {
			c.log.Error("Fetch smart contract failed, failed to create smart contract folder", "err", err)
			return
		}
		fetchSC.SmartContractTokenPath, err = c.RenameSCFolder(fetchSC.SmartContractTokenPath, smartContractToken)
		if err != nil {
			c.log.Error("Fetch smart contract failed, failed to create SC folder", "err", err)
			return
		}
		c.FetchSmartContract(requestID, &fetchSC)
		c.log.Info("Smart contract " + smartContractToken + " files fetching successful")
	}
	publisherPeerID := peerID
	did := newEvent.Did
	address := publisherPeerID + "." + did
	p, err := c.getPeer(address)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return
	}
	err, _ = c.syncTransactionChainFrom(p, "", smartContractToken)
	if err != nil {
		c.log.Error("Failed to sync token chain block", "err", err)
		return
	}
	c.log.Info("Token chain of " + smartContractToken + " syncing successful")
	curlUrl, err := c.w.GetSmartContractTokenUrl(smartContractToken)
	if err != nil {
		c.log.Error("Failed to get smart contract token URL", "err", err)
		return
	}
	payload := map[string]interface{}{
		"smart_contract_hash": newEvent.SmartContractToken,
		"port":                c.cfg.NodePort,
		"smart_contract_data": newEvent.SmartContractData,
		"initiator_did":       newEvent.Did,
	}

	payLoadBytes, err := json.Marshal(payload)
	if err != nil {
		c.log.Error("Failed to marshal JSON", "err", err)
		return
	}
	request, err := http.NewRequest("POST", curlUrl, bytes.NewBuffer(payLoadBytes))
	if err != nil {
		fmt.Println("Error creating HTTP request for smart contract statefile updationcallback: ", err)
		return
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		fmt.Println("Error sending HTTP request for smart contract statefile updation: ", err)
		return
	}
	if response.StatusCode != http.StatusOK {
		c.log.Error("Error getting response from SC", "status", response.Status)
		return
	}
	responseBodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %s\n", err)
		return
	}
	responseBody := string(responseBodyBytes)
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &responseData); err != nil {
		c.log.Error("Error parsing JSON:", err)
		return
	}
	message, ok := responseData["message"].(string)
	if !ok {
		c.log.Error("Error: 'message' field not found or not a string")
		return
	}
	c.log.Debug(message)
	defer response.Body.Close()
}

// RegisterCallBackURL registers a callback URL for smart contract events.
func (c *Core) RegisterCallBackURL(req *model.RegisterCallBackUrlReq) *model.BasicResponse {
	// Validate input
	if req.SmartContractToken == "" {
		return &model.BasicResponse{Status: false, Message: "smart contract token is required"}
	}
	if req.CallBackURL == "" {
		return &model.BasicResponse{Status: false, Message: "callback URL is required"}
	}

	// Register the callback URL in the database
	err := c.w.RegisterCallbackURL(req.SmartContractToken, req.CallBackURL)
	if err != nil {
		c.log.Error("Failed to register callback URL", "smart_contract", req.SmartContractToken, "err", err)
		return &model.BasicResponse{Status: false, Message: fmt.Sprintf("failed to register callback URL: %v", err)}
	}

	c.log.Info("Callback URL registered successfully", "smart_contract", req.SmartContractToken, "callback_url", req.CallBackURL)
	return &model.BasicResponse{Status: true, Message: "callback URL registered successfully"}
}

func (c *Core) GetAllSmartcontracts() ([]models.Token, error) {
	smartContracts, err := c.w.GetSmartContractTokens()
	if err != nil {
		return nil, err
	}
	return smartContracts, nil
}

func (c *Core) GetSmartContractChain(smartContractID string) ([]models.TokenChainResponse, error) {
	smartContractTokenChain, err := c.w.GetSmartContractChainByTokenID(smartContractID)
	if err != nil {
		return nil, err
	}
	return smartContractTokenChain, nil
}
