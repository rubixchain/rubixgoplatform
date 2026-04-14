package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/rubixchain/rubixgoplatform/core/model"
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
	ConOwnerDID string `json:"contract_ownwer_did"`
	ConHash     string `json:"contract_hash"`
}

var reqID string

type GenerateSmartContractRequest struct {
	BinaryCode string
	RawCode    string
	DID        string
	SCPath     string
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

// generateSmartContractToken uses folder-based IPFS approach (same as NFT)
// Adds the entire smart contract folder to IPFS instead of individual files
func (c *Core) generateSmartContractToken(requestID string, smartContractTokenRequest *GenerateSmartContractRequest) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}

	// Add entire smart contract folder to IPFS (contains .wasm and .rs files)
	scFolderHash, err := c.ipfsOps.AddDir(smartContractTokenRequest.SCPath)
	if err != nil {
		c.log.Error("Failed to add smart contract folder to IPFS", "err", err)
		return basicResponse
	}

	// Create data pointing to the folder hash
	// This was SmartContractToken before which is updated to IPFSContractInfo
	// So that this struct remains same for NFTs and SmartContracts
	smartContractToken := models.IPFSContractInfo{
		ArtifactHash: scFolderHash,
		DID:          smartContractTokenRequest.DID,
		PeerID:       c.peerID,
	}

	// Marshal metadata to JSON
	smartContractTokenJSON, err := json.MarshalIndent(smartContractToken, "", "  ")
	if err != nil {
		c.log.Error("Failed to marshal SmartContractToken struct", "err", err)
		return basicResponse
	}

	// Add metadata JSON to IPFS - this hash becomes the token ID
	smartContractTokenHash, err := IpfsAddWithBackoff(c.ipfs, bytes.NewReader(smartContractTokenJSON))
	if err != nil {
		c.log.Error("Failed to add SmartContractToken to IPFS", "err", err)
		return basicResponse
	}

	c.log.Info("Smart contract token hash generated", "token_hash", smartContractTokenHash)

	// Set the response status and message
	smartContractTokenResponse := &SmartContractTokenResponse{
		Message: "Smart contract generated successfully",
		Result:  smartContractTokenHash,
	}

	// Rename folder from temp UUID to token hash
	_, err = c.RenameSCFolder(smartContractTokenRequest.SCPath, smartContractTokenHash)
	if err != nil {
		c.log.Error("Failed to rename SC folder", "err", err)
		return basicResponse
	}

	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = smartContractTokenResponse.Message
	basicResponse.Result = smartContractTokenResponse.Result

	return basicResponse
}

// FetchSmartContract fetches a smart contract from IPFS and stores it locally.
// It handles the complete lifecycle: folder management, metadata fetching, file storage, and token chain sync.
func (c *Core) FetchSmartContract(requestID string, smartContractToken string) *model.BasicResponse {
	c.log.Info("FetchSmartContract: Fetching smart contract", "request_id", requestID, "token", smartContractToken)

	scFolderPath, err := c.prepareSmartContractFolder(smartContractToken)
	if err != nil {
		c.log.Error("FetchSmartContract: Failed to prepare smart contract folder", "token", smartContractToken, "err", err)
		return &model.BasicResponse{Status: false, Message: err.Error()}
	}

	metadata, err := c.fetchContractInfo(smartContractToken)
	if err != nil {
		c.log.Error("FetchSmartContract: Failed to fetch smart contract metadata", "token", smartContractToken, "err", err)
		return &model.BasicResponse{Status: false, Message: err.Error()}
	}

	if err := c.storeSmartContractFiles(scFolderPath, metadata); err != nil {
		c.log.Error("FetchSmartContract: Failed to store smart contract files", "token", smartContractToken, "err", err)
		return &model.BasicResponse{Status: false, Message: err.Error()}
	}

	if err := c.syncSmartContractTransaction(smartContractToken, metadata); err != nil {
		c.log.Warn("FetchSmartContract: Failed to sync transaction chain", "token", smartContractToken, "err", err)
		return &model.BasicResponse{Status: false, Message: err.Error()}
	}

	c.log.Info("FetchSmartContract: Successfully fetched smart contract", "token", smartContractToken)
	return &model.BasicResponse{
		Status:  true,
		Message: "Successfully fetched smart contract",
		Result:  metadata,
	}
}

// prepareSmartContractFolder ensures the smart contract folder is ready.
// It removes any existing folder and creates a new one.
func (c *Core) prepareSmartContractFolder(smartContractToken string) (string, error) {
	scFolderPath := path.Join(c.smartContractDir, smartContractToken)

	// Remove existing folder if present
	if _, err := os.Stat(scFolderPath); err == nil {
		c.log.Debug("Removing existing smart contract folder", "path", scFolderPath)
		if err := os.RemoveAll(scFolderPath); err != nil {
			return "", fmt.Errorf("failed to remove existing folder: %w", err)
		}
	}

	// Create fresh folder
	if err := os.MkdirAll(scFolderPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create smart contract folder: %w", err)
	}

	return scFolderPath, nil
}

// storeSmartContractFiles downloads the entire smart contract folder from IPFS using the folder hash.
func (c *Core) storeSmartContractFiles(scFolderPath string, metadata *models.IPFSContractInfo) error {
	// Get the entire folder from IPFS using the artifact hash
	err := c.ipfsOps.Get(metadata.ArtifactHash, scFolderPath)
	if err != nil {
		return fmt.Errorf("failed to fetch smart contract folder from IPFS: %w", err)
	}

	return nil
}

// syncSmartContractTokenChain syncs the token chain from the deployer's peer if available.
func (c *Core) syncSmartContractTransaction(smartContractToken string, metadata *models.IPFSContractInfo) error {
	if metadata.PeerID == "" {
		c.log.Debug("syncSmartContractTransaction: No peer ID available, skipping token chain sync", "token", smartContractToken)
		return nil
	}

	peerAddr := metadata.PeerID + "." + metadata.DID
	if err := c.SyncTransactionChainsFromPeer(peerAddr, []string{smartContractToken}, nil, nil, false); err != nil {
		c.log.Error("syncSmartContractTransaction: Failed to sync transaction chain", "token", smartContractToken, "err", err)
		return fmt.Errorf("syncSmartContractTransaction: failed to sync transaction chain: %w", err)
	}

	c.log.Debug("syncSmartContractTransaction: Successfully synced transaction chain", "token", smartContractToken)
	return nil
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
	c.log.Info("publishSmartContractEvents: Publishing events for smart contracts",
		"transactionID", transactionId,
		"initiator", initiatorDID,
		"epoch", epoch,
		"smartContractCount", len(smartContracts),
	)

	baseEvent := models.EventSmartContractPublishInfo{
		TransactionID:      transactionId,
		Initiator:          initiatorDID,
		InitiatorSignature: initiatorSignature,
		Epoch:              epoch,
	}

	for i, sc := range smartContracts {

		event := baseEvent
		event.SmartContractID = sc.SmartContractId
		event.SmartContractData = sc.Data

		c.log.Info("publishSmartContractEvents: Publishing event",
			"index", i,
			"smartContractID", sc.SmartContractId,
			"hasData", sc.Data != "",
			"transactionID", transactionId,
		)

		if err := c.publishNewSmartContractEvent(&event); err != nil {
			c.log.Error("publishSmartContractEvents: Smart contract event publish failed",
				"smartcontract", sc.SmartContractId,
				"err", err,
			)
		} else {
			c.log.Info("publishSmartContractEvents: Event published successfully",
				"smartContractID", sc.SmartContractId,
				"topic", sc.SmartContractId,
			)
		}
	}
}

func (c *Core) SubsribeContractSetup(requestID string, topic string) error {
	reqID = requestID

	// Subscribe to smart contract topic
	err := c.ps.SubscribeTopic(topic, c.ContractCallBack)
	if err != nil {
		c.log.Error("Failed to subscribe to smart contract topic", "topic", topic, "err", err)
		return fmt.Errorf("failed to subscribe to smart contract topic %s: %w", topic, err)
	}

	// Check if smart contract folder exists
	scFolderPath := path.Join(c.smartContractDir, topic)
	if _, err := os.Stat(scFolderPath); os.IsNotExist(err) {
		c.log.Info("Smart contract not found locally, fetching from network", "token", topic)

		// Fetch smart contract from IPFS
		response := c.FetchSmartContract(reqID, topic)
		if !response.Status {
			c.log.Error("Failed to fetch smart contract", "token", topic, "error", response.Message)
			return fmt.Errorf("failed to fetch smart contract %s: %s", topic, response.Message)
		}

		c.log.Info("Smart contract fetched successfully", "token", topic)
	} else {
		c.log.Debug("Smart contract already exists locally", "token", topic, "path", scFolderPath)
	}

	c.log.Info("Successfully subscribed to smart contract", "topic", topic)
	return nil
}

// ContractCallBack handles pubsub notifications for smart contract events.
// IMPORTANT: The publisher sends models.EventSmartContractPublishInfo — we must
// deserialize into the SAME struct. Previously this used model.NewContractEvent
// whose JSON tags did not match, causing all fields to unmarshal as zero values
// and silently breaking the sync flow.
func (c *Core) ContractCallBack(peerID string, topic string, data []byte) {
	c.log.Info("ContractCallBack: Received pubsub message",
		"peerID", peerID,
		"topic", topic,
		"dataLen", len(data),
		"rawData", string(data),
	)

	var newEvent models.EventSmartContractPublishInfo

	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("ContractCallBack: Failed to unmarshal contract event", "err", err, "rawData", string(data))
		return
	}

	c.log.Info("ContractCallBack: Parsed smart contract event",
		"smartContractID", newEvent.SmartContractID,
		"transactionID", newEvent.TransactionID,
		"initiator", newEvent.Initiator,
		"epoch", newEvent.Epoch,
		"hasData", newEvent.SmartContractData != "",
	)

	smartContractToken := newEvent.SmartContractID
	if smartContractToken == "" {
		c.log.Error("ContractCallBack: SmartContractID is empty after unmarshal — cannot proceed",
			"topic", topic, "peerID", peerID, "rawData", string(data))
		return
	}

	scFolderPath := path.Join(c.smartContractDir, smartContractToken)

	if _, err := os.Stat(scFolderPath); os.IsNotExist(err) {
		c.log.Warn("ContractCallBack: Smart contract folder does not exist", "token", smartContractToken, "path", scFolderPath)
	} else {
		c.log.Debug("ContractCallBack: Smart contract folder exists", "token", smartContractToken, "path", scFolderPath)
	}

	initiatorDID := newEvent.Initiator
	if initiatorDID == "" {
		c.log.Error("ContractCallBack: Initiator DID is empty — cannot construct peer address",
			"topic", topic, "peerID", peerID)
		return
	}

	address := peerID + "." + initiatorDID
	c.log.Info("ContractCallBack: Syncing transaction chain from publisher",
		"token", smartContractToken,
		"peerAddress", address,
	)

	if err := c.SyncTransactionChainsFromPeer(address, []string{smartContractToken}, nil, nil, false); err != nil {
		c.log.Error("ContractCallBack: Failed to sync transaction chain",
			"token", smartContractToken,
			"peerAddress", address,
			"err", err,
		)
		return
	}

	c.log.Info("ContractCallBack: Transaction chain synced successfully", "token", smartContractToken)

	curlUrl, err := c.w.GetSmartContractTokenUrl(smartContractToken)
	if err != nil {
		c.log.Error("ContractCallBack: Failed to get smart contract callback URL", "token", smartContractToken, "err", err)
		return
	}

	c.log.Debug("ContractCallBack: Sending callback HTTP request", "url", curlUrl, "token", smartContractToken)

	payload := map[string]interface{}{
		"smart_contract_hash": smartContractToken,
		"port":                c.cfg.NodePort,
		"smart_contract_data": newEvent.SmartContractData,
		"initiator_did":       initiatorDID,
	}

	payLoadBytes, err := json.Marshal(payload)
	if err != nil {
		c.log.Error("ContractCallBack: Failed to marshal callback payload", "err", err)
		return
	}

	request, err := http.NewRequest("POST", curlUrl, bytes.NewBuffer(payLoadBytes))
	if err != nil {
		c.log.Error("ContractCallBack: Failed to create HTTP request", "url", curlUrl, "err", err)
		return
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		c.log.Error("ContractCallBack: Failed to send HTTP request to callback URL", "url", curlUrl, "err", err)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		c.log.Error("ContractCallBack: Received non-OK status from callback", "status", response.Status, "url", curlUrl)
		return
	}

	responseBodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		c.log.Error("ContractCallBack: Error reading response body", "err", err)
		return
	}

	var responseData map[string]interface{}
	if err := json.Unmarshal(responseBodyBytes, &responseData); err != nil {
		c.log.Error("ContractCallBack: Error parsing callback response JSON", "err", err)
		return
	}

	message, ok := responseData["message"].(string)
	if !ok {
		c.log.Error("ContractCallBack: 'message' field not found or not a string in callback response")
		return
	}

	c.log.Info("ContractCallBack: Callback response received", "message", message, "token", smartContractToken)
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

// fetchContractInfo is a unified function to fetch metadata for both NFTs and Smart Contracts.
// It retrieves the IPFS metadata JSON and parses it into models.IPFSContractInfo.
// This function works for both NFTs and Smart Contracts since they now share the same metadata structure.
func (c *Core) fetchContractInfo(tokenHash string) (*models.IPFSContractInfo, error) {
	// Fetch metadata JSON from IPFS
	reader, err := c.ipfsOps.Cat(tokenHash)
	if err != nil {
		return nil, fmt.Errorf("fetchContractInfo: failed to fetch contract metadata from IPFS: %w", err)
	}
	defer reader.Close()

	// Read JSON bytes
	metadataBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("fetchContractInfo: failed to read contract metadata: %w", err)
	}

	// Parse into unified struct
	var contractInfo models.IPFSContractInfo
	if err := json.Unmarshal(metadataBytes, &contractInfo); err != nil {
		return nil, fmt.Errorf("fetchContractInfo: failed to parse contract metadata JSON: %w", err)
	}

	return &contractInfo, nil
}
