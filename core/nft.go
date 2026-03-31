package core

// nft.go — dead-code stub (Phase 09 replacement target)
// NFT creation, deployment, and transfer logic will be replaced by InitiateTransaction.
// These stubs satisfy server/ call sites until the replacement is wired.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// NFTReq holds the inputs for creating a new NFT.
type NFTReq struct {
	DID      string
	Metadata string
	Artifact string
	NFTPath  string
}

// FetchNFTRequest is the request payload for fetching an NFT.
type FetchNFTRequest struct {
	NFT     string
	NFTPath string
}

func (c *Core) CreateNFTRequest(requestID string, createNFTRequest NFTReq) {
	defer os.RemoveAll(createNFTRequest.NFTPath)
	createNFTResponse := c.createNFT(requestID, createNFTRequest)
	didChannel := c.GetWebReq(requestID)
	if didChannel == nil {
		c.log.Error("failed to get web request", "requestID", requestID)
	}
	didChannel.OutChan <- createNFTResponse
}

func (c *Core) createNFT(requestID string, createNFTRequest NFTReq) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}
	nftFolderHash, err := c.ipfsOps.AddDir(createNFTRequest.NFTPath)
	if err != nil {
		c.log.Error("Failed to add nft file to IPFS", "err", err)
		return basicResponse
	}
	// This was previously NFTIpfsInfo which we have updated to IPFSContractInfo
	// The idea was we keep this struct same for NFTs and contracts
	nft := models.IPFSContractInfo{
		DID:          createNFTRequest.DID,
		ArtifactHash: nftFolderHash,
		PeerID:       c.peerID,
	}

	nftJSON, err := json.MarshalIndent(nft, "", "  ")
	if err != nil {
		c.log.Error("Failed to marshal nft struct", "err", err)
		return basicResponse
	}

	nftHash, err := IpfsAddWithBackoff(c.ipfs, bytes.NewReader(nftJSON))
	if err != nil {
		c.log.Error("Failed to add nft to IPFS", "err", err)
		return basicResponse
	}

	c.log.Info("The NFT token hash generated ", nftHash)

	// Set the response status and message
	nftTokenResponse := &SmartContractTokenResponse{
		Message: "NFT Token generated successfully",
		Result:  nftHash,
	}

	_, err = c.RenameNFTFolder(createNFTRequest.NFTPath, nftHash)
	if err != nil {
		c.log.Error("Failed to rename NFT folder", "err", err)
		return basicResponse
	}

	basicResponse.Status = true
	basicResponse.Message = nftTokenResponse.Message
	basicResponse.Result = nftTokenResponse.Result

	return basicResponse
}

func (c *Core) DeployNFT(reqID string, deployReq model.DeployNFTRequest) {
	br := c.deployNFT(reqID, deployReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) deployNFT(reqID string, deployReq model.DeployNFTRequest) *model.BasicResponse {
	// st := time.Now()
	// txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}
	_, did, ok := util.ParseAddress(deployReq.DID)
	if !ok {
		resp.Message = "Invalid Deployer DID"
		return resp
	}

	// isNFT, err := c.w.IsNFT(deployReq.NFT)
	// if err != nil {
	// 	resp.Message = "deployNFT : The TokenID given is not an NFT"
	// 	return resp
	// }
	// if !isNFT {
	// 	resp.Message = "deployNFT : The TokenID given is not an NFT"
	// 	return resp
	// }

	// This part need to be verified.
	//Here we are querying the db and checkign whether the NFT has already been deployed or not.
	// Need to ensure whether this itself is the proper approach : This was the approach we were doing previously
	_, err := c.w.GetTokenByTokenID(deployReq.NFT)
	if err == nil {
		c.log.Error(fmt.Sprintf("NFT %v has been already been deployed", deployReq.NFT))
		resp.Message = fmt.Sprintf("NFT %v has already been deployed", deployReq.NFT)
		return resp
	}

	_, err = c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Deployer DID of the NFT deployer, " + err.Error()
		return resp
	}

	// Building of the TransactionInfo and

	nftTokenDetails := wallet.NFT{
		TokenID:     deployReq.NFT,
		DID:         deployReq.DID,
		TokenStatus: wallet.TokenIsFree,
		TokenValue:  floatPrecision(deployReq.NFTValue, MaxDecimalPlaces),
		Metadata:    deployReq.NFTMetadata,
		Filename:    deployReq.NFTFileName,
	}

	if err := c.w.CreateNFT(&nftTokenDetails, false); err != nil {
		c.log.Error("Failed to write nft to storage in NFTTokenStorage", err)
		return resp
	}

	newEvent := model.NFTEvent{
		NFT:         nftTokenDetails.TokenID,
		ExecutorDid: nftTokenDetails.DID,
		NFTMetadata: nftTokenDetails.Metadata,
		NFTFileName: nftTokenDetails.Filename,
		NFTValue:    nftTokenDetails.TokenValue,
	}

	err = c.publishNewNftEvent(&newEvent)
	if err != nil {
		c.log.Error("Failed to publish NFT info")
	}

	c.log.Info("NFT Deployed successfully")
	resp.Status = true
	msg := fmt.Sprintf("NFT Deployed successfully")
	resp.Message = msg
	return resp
}

// GetAllNFT returns an empty NFT list stub.
func (c *Core) GetAllNFT() model.NFTList {
	return model.NFTList{}
}

// GetNFTsByDid returns an empty NFT list stub for a given DID.
func (c *Core) GetNFTsByDid(did string) model.NFTList {
	return model.NFTList{}
}

func (c *Core) SubscribeNFTSetup(requestID string, topic string) error {
	reqID = requestID

	// Subscribe to NFT topic
	err := c.ps.SubscribeTopic(topic, c.NFTCallBack)
	if err != nil {
		c.log.Error("SubscribeNFTSetup: Failed to subscribe to NFT topic", "topic", topic, "err", err)
		return fmt.Errorf("failed to subscribe to NFT topic %s: %w", topic, err)
	}

	// Check if NFT folder exists
	nftFolderPath := path.Join(c.nftDir, topic)
	if _, err := os.Stat(nftFolderPath); os.IsNotExist(err) {
		c.log.Info("SubscribeNFTSetup: NFT not found locally, fetching from network", "nft_token", topic)

		// Fetch NFT from IPFS
		fetchRequest := &FetchNFTRequest{
			NFT: topic,
		}
		response := c.FetchNFT(fetchRequest)
		if !response.Status {
			c.log.Error("SubscribeNFTSetup: Failed to fetch NFT", "nft_token", topic, "error", response.Message)
			return fmt.Errorf("failed to fetch NFT %s: %s", topic, response.Message)
		}

		c.log.Info("SubscribeNFTSetup: NFT fetched successfully", "nft_token", topic)
	} else {
		c.log.Debug("SubscribeNFTSetup: NFT already exists locally", "nft_token", topic, "path", nftFolderPath)
	}

	c.log.Info("SubscribeNFTSetup: Successfully subscribed to NFT", "topic", topic)
	return nil
}

func (c *Core) NFTCallBack(peerID string, topic string, data []byte) {
	var newEvent model.NFTEvent
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("NFTCallBack: Failed to unmarshal NFT event", "err", err)
		return
	}

	nft := newEvent.NFT
	c.log.Info("NFTCallBack: Received update on NFT", "nft_token", nft)

	// Check if NFT folder exists (log warning if missing)
	nftFolderPath := path.Join(c.nftDir, nft)
	if _, err := os.Stat(nftFolderPath); os.IsNotExist(err) {
		c.log.Warn("NFTCallBack: NFT folder does not exist", "nft_token", nft)
	}

	// Construct publisher peer address
	executorDid := newEvent.ExecutorDid
	publisherAddress := peerID + "." + executorDid
	publisherPeer, err := c.getPeer(publisherAddress)
	if err != nil {
		c.log.Error("NFTCallBack: Failed to get peer", "address", publisherAddress, "err", err)
		return
	}

	// Sync transaction chain from publisher
	err, _ = c.syncTransactionChainFrom(publisherPeer, "", nft)
	if err != nil {
		c.log.Error("NFTCallBack: Failed to sync transaction chain", "nft_token", nft, "err", err)
		return
	}

	c.log.Info("NFTCallBack: Transaction chain synced successfully", "nft_token", nft)
}

func (c *Core) FetchNFT(fetchNFTRequest *FetchNFTRequest) *model.BasicResponse {
	c.log.Info("FetchNFT: Starting NFT fetch", "nft_token", fetchNFTRequest.NFT)

	basicResponse := &model.BasicResponse{
		Status: false,
	}

	// Step 1: Fetch NFT metadata from IPFS
	nftJSON, err := c.ipfsOps.Cat(fetchNFTRequest.NFT)
	if err != nil {
		c.log.Error("FetchNFT: Failed to get NFT from network", "nft_token", fetchNFTRequest.NFT, "err", err)
		basicResponse.Message = "Failed to get NFT details from network"
		return basicResponse
	}

	nftJSONBytes, err := io.ReadAll(nftJSON)
	if err != nil {
		c.log.Error("FetchNFT: Failed to read NFT from network", "nft_token", fetchNFTRequest.NFT, "err", err)
		basicResponse.Message = "Failed to read NFT from network"
		return basicResponse
	}
	nftJSON.Close()

	// Step 2: Parse NFT metadata
	var nft NFTIpfsInfo
	err = json.Unmarshal(nftJSONBytes, &nft)
	if err != nil {
		c.log.Error("FetchNFT: Failed to parse NFT metadata", "nft_token", fetchNFTRequest.NFT, "err", err)
		basicResponse.Message = "Failed to parse NFT metadata"
		return basicResponse
	}
	c.log.Info("FetchNFT: Successfully parsed NFT metadata", "nft_token", fetchNFTRequest.NFT, "did", nft.DID, "peer_id", nft.PeerID)

	// Step 3: Fetch NFT artifact files from IPFS
	err = c.GetNFTFromIpfs(fetchNFTRequest.NFT, nft.ArtifactHash)
	if err != nil {
		c.log.Error("FetchNFT: Failed to fetch NFT files from IPFS", "nft_token", fetchNFTRequest.NFT, "artifact_hash", nft.ArtifactHash, "err", err)
		basicResponse.Message = "Failed to fetch NFT files from IPFS"
		return basicResponse
	}
	c.log.Info("FetchNFT: Successfully fetched NFT artifact files", "nft_token", fetchNFTRequest.NFT, "artifact_hash", nft.ArtifactHash)

	// Step 4: Sync transaction chain if PeerID is available
	if nft.PeerID != "" {
		err = c.syncNFTTransaction(fetchNFTRequest.NFT, &nft)
		if err != nil {
			c.log.Error("FetchNFT: Failed to sync NFT transaction chain", "nft_token", fetchNFTRequest.NFT, "err", err)
			basicResponse.Message = "Failed to sync NFT transaction chain"
			return basicResponse
		}
		c.log.Info("FetchNFT: Successfully synced NFT transaction chain", "nft_token", fetchNFTRequest.NFT)
	} else {
		c.log.Info("FetchNFT: Skipping transaction chain sync (no PeerID in metadata)", "nft_token", fetchNFTRequest.NFT)
	}

	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = "NFT fetched successfully"
	basicResponse.Result = &nft
	c.log.Info("FetchNFT: NFT fetch completed successfully", "nft_token", fetchNFTRequest.NFT)

	return basicResponse
}

// syncNFTTransaction syncs the transaction chain for an NFT from the deployer peer
func (c *Core) syncNFTTransaction(nftToken string, nft *NFTIpfsInfo) error {
	c.log.Info("syncNFTTransaction: Starting transaction chain sync", "nft_token", nftToken, "peer_id", nft.PeerID, "did", nft.DID)

	// Construct the peer address
	address := nft.PeerID + "." + nft.DID
	peer, err := c.getPeer(address)
	if err != nil {
		c.log.Error("syncNFTTransaction: Failed to get peer", "nft_token", nftToken, "address", address, "err", err)
		return fmt.Errorf("failed to get peer %s: %w", address, err)
	}
	c.log.Info("syncNFTTransaction: Successfully retrieved peer", "nft_token", nftToken, "address", address)

	// Sync the transaction chain from the peer
	err, _ = c.syncTransactionChainFrom(peer, "", nftToken)
	if err != nil {
		c.log.Error("syncNFTTransaction: Failed to sync transaction chain", "nft_token", nftToken, "peer", address, "err", err)
		return fmt.Errorf("failed to sync transaction chain for NFT %s: %w", nftToken, err)
	}

	c.log.Info("syncNFTTransaction: Transaction chain sync completed successfully", "nft_token", nftToken)
	return nil
}

func (c *Core) publishNewNftEvent(newNFTEvent *models.EventNFTPublishInfo) error {
	if c.ps == nil {
		return nil
	}

	topic := newNFTEvent.NFTid

	if err := c.ps.Publish(topic, newNFTEvent); err != nil {
		c.log.Error("Failed to publish NFT event", "topic", topic, "err", err)
		return err
	}

	c.log.Info("New state published on NFT ", "topic", topic)

	return nil
}

func (c *Core) publishNFTEvents(
	request *models.TransactionRequest,
	transactionId string,
	initiatorDID string,
	initiatorSignature string,
	epoch int,
) {

	nfts := request.GetAllNFTs()

	baseEvent := models.EventNFTPublishInfo{
		TransactionID:      transactionId,
		Initiator:          initiatorDID,
		InitiatorSignature: initiatorSignature,
		Epoch:              epoch,
	}

	for _, nft := range nfts {

		event := baseEvent
		event.NFTid = nft.NFTId
		event.NFTData = nft.Data

		if err := c.publishNewNftEvent(&event); err != nil {
			c.log.Error("NFT event publish failed",
				"nft", nft.NFTId,
				"err", err,
			)
		}
	}
}

// CheckNFTFolderExists stubs NFT folder existence check.
func (c *Core) CheckNFTFolderExists(nft string) (string, error) {
	return "", fmt.Errorf("CheckNFTFolderExists: not implemented")
}

// GetAllNFTs stubs listing all NFTs.
func (c *Core) GetAllNFTs() ([]models.FullNodeNFT, error) {
	return nil, fmt.Errorf("GetAllNFTs: not implemented")
}

// GetNFTChain stubs retrieval of NFT token chain data.
func (c *Core) GetNFTChain(nftID string) ([]models.TokenChainResponse, error) {
	return nil, fmt.Errorf("GetNFTChain: not implemented")
}

// DumpTokenChain stubs a token chain dump operation.
func (c *Core) DumpTokenChain(req *model.TCDumpRequest) *model.TCDumpReply {
	return &model.TCDumpReply{}
}
