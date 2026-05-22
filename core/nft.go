package core

// nft.go — dead-code stub (Phase 09 replacement target)
// NFT creation, deployment, and transfer logic will be replaced by InitiateTransaction.
// These stubs satisfy server/ call sites until the replacement is wired.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
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

// ensureNFTArtifactAndSubscription fetches IPFS artifacts (if missing) and
// subscribes to the NFT topic. Skips chain sync — callers handle that
// separately to avoid double-syncing in the receiver-transfer flow.
func (c *Core) ensureNFTArtifactAndSubscription(nftID string) error {
	if nftID == "" {
		return fmt.Errorf("ensureNFTArtifactAndSubscription: empty nftID")
	}

	nftFolderPath := path.Join(c.nftDir, nftID)
	if _, err := os.Stat(nftFolderPath); os.IsNotExist(err) {
		c.log.Info("ensureNFTArtifactAndSubscription: NFT not found locally, fetching from network", "nft_token", nftID)

		nft, err := c.fetchContractInfo(nftID)
		if err != nil {
			c.log.Error("ensureNFTArtifactAndSubscription: failed to fetch NFT metadata", "nft_token", nftID, "err", err)
			return fmt.Errorf("ensureNFTArtifactAndSubscription: failed to fetch NFT metadata for %s: %w", nftID, err)
		}

		if err := c.GetNFTFromIpfs(nftID, nft.ArtifactHash); err != nil {
			c.log.Error("ensureNFTArtifactAndSubscription: failed to fetch NFT artifact from IPFS", "nft_token", nftID, "artifact_hash", nft.ArtifactHash, "err", err)
			return fmt.Errorf("ensureNFTArtifactAndSubscription: failed to fetch NFT artifact for %s: %w", nftID, err)
		}

		c.log.Info("ensureNFTArtifactAndSubscription: NFT fetched successfully", "nft_token", nftID)
	} else {
		c.log.Debug("ensureNFTArtifactAndSubscription: NFT folder already exists, skipping fetch", "nft_token", nftID, "path", nftFolderPath)
	}

	if err := c.ps.SubscribeTopic(nftID, c.NFTCallBack); err != nil {
		if err.Error() == "topic already subscribed" {
			c.log.Debug("ensureNFTArtifactAndSubscription: already subscribed to NFT topic", "topic", nftID)
		} else {
			c.log.Error("ensureNFTArtifactAndSubscription: failed to subscribe to NFT topic", "topic", nftID, "err", err)
			return fmt.Errorf("ensureNFTArtifactAndSubscription: failed to subscribe to NFT topic %s: %w", nftID, err)
		}
	}

	return nil
}

func (c *Core) SubscribeNFTSetup(requestID string, topic string) error {
	reqID = requestID

	if err := c.ensureNFTArtifactAndSubscription(topic); err != nil {
		return err
	}

	// Explicit subscribers need the existing chain; helper skips chain sync.
	nft, err := c.fetchContractInfo(topic)
	if err != nil {
		c.log.Warn("SubscribeNFTSetup: chain sync metadata lookup failed (continuing)", "nft_token", topic, "err", err)
	} else if nft.PeerID != "" {
		if err := c.syncNFTTransaction(topic, nft); err != nil {
			c.log.Warn("SubscribeNFTSetup: chain sync failed (will rely on future pubsub events)", "nft_token", topic, "err", err)
		}
	}

	c.log.Info("SubscribeNFTSetup: Successfully subscribed to NFT", "topic", topic)
	return nil
}

func (c *Core) NFTCallBack(peerID string, topic string, data []byte) {
	c.log.Info("NFTCallBack: Received pubsub message",
		"peerID", peerID,
		"topic", topic,
		"dataLen", len(data),
		"rawData", string(data),
	)

	// Skip self-echo: when the node publishes an NFT event, it may receive
	// its own message back via pubsub. Syncing from ourselves would race
	// with the ongoing transaction's persistence and potentially corrupt
	// token state.
	if peerID == c.peerID {
		c.log.Debug("NFTCallBack: Ignoring self-published event", "topic", topic)
		return
	}

	var newEvent models.EventNFTPublishInfo
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("NFTCallBack: Failed to unmarshal NFT event", "err", err, "rawData", string(data))
		return
	}

	nft := newEvent.NFTid
	c.log.Info("NFTCallBack: Parsed NFT event",
		"nftID", nft,
		"transactionID", newEvent.TransactionID,
		"initiator", newEvent.Initiator,
		"epoch", newEvent.Epoch,
		"hasData", newEvent.NFTData != "",
	)

	if nft == "" {
		c.log.Error("NFTCallBack: NFTid is empty after unmarshal — cannot proceed",
			"topic", topic, "peerID", peerID, "rawData", string(data))
		return
	}

	// Check if NFT folder exists (log warning if missing)
	nftFolderPath := path.Join(c.nftDir, nft)
	if _, err := os.Stat(nftFolderPath); os.IsNotExist(err) {
		c.log.Warn("NFTCallBack: NFT folder does not exist", "nft_token", nft, "path", nftFolderPath)
	} else {
		c.log.Debug("NFTCallBack: NFT folder exists", "nft_token", nft, "path", nftFolderPath)
	}

	// Construct publisher peer address
	initiatorDid := newEvent.Initiator
	if initiatorDid == "" {
		c.log.Error("NFTCallBack: Initiator DID is empty — cannot construct peer address",
			"topic", topic, "peerID", peerID)
		return
	}

	publisherAddress := peerID + "." + initiatorDid
	c.log.Info("NFTCallBack: Syncing transaction chain from publisher",
		"nft_token", nft,
		"peerAddress", publisherAddress,
	)

	// Sync transaction chain from publisher
	if err := c.SyncTransactionChainsFromPeer(publisherAddress, []string{nft}, nil, nil, newEvent.NFTOwnershipTransfer, false); err != nil {
		c.log.Error("NFTCallBack: Failed to sync transaction chain",
			"nft_token", nft,
			"peerAddress", publisherAddress,
			"err", err,
		)
		return
	}

	c.log.Info("NFTCallBack: Transaction chain synced successfully", "nft_token", nft)
}

func (c *Core) FetchNFT(fetchNFTRequest *FetchNFTRequest) *model.BasicResponse {
	c.log.Info("FetchNFT: Starting NFT fetch", "nft_token", fetchNFTRequest.NFT)

	basicResponse := &model.BasicResponse{
		Status: false,
	}

	// Step 1: Fetch NFT metadata from IPFS using unified function
	nft, err := c.fetchContractInfo(fetchNFTRequest.NFT)
	if err != nil {
		c.log.Error("FetchNFT: Failed to fetch NFT metadata", "nft_token", fetchNFTRequest.NFT, "err", err)
		basicResponse.Message = fmt.Sprintf("Failed to fetch NFT metadata: %v", err)
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
		err = c.syncNFTTransaction(fetchNFTRequest.NFT, nft)
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
func (c *Core) syncNFTTransaction(nftToken string, nft *models.IPFSContractInfo) error {
	c.log.Info("syncNFTTransaction: Starting transaction chain sync", "nft_token", nftToken, "peer_id", nft.PeerID, "did", nft.DID)

	address := nft.PeerID + "." + nft.DID
	if err := c.SyncTransactionChainsFromPeer(address, []string{nftToken}, nil, nil, false, false); err != nil {
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
	c.log.Info("publishNFTEvents: Publishing events for NFTs",
		"transactionID", transactionId,
		"initiator", initiatorDID,
		"epoch", epoch,
		"nftCount", len(nfts),
	)

	baseEvent := models.EventNFTPublishInfo{
		TransactionID:      transactionId,
		Initiator:          initiatorDID,
		InitiatorSignature: initiatorSignature,
		Epoch:              epoch,
	}

	for i, nft := range nfts {

		event := baseEvent
		event.NFTid = nft.NFTId
		event.NFTData = nft.Data
		event.NFTOwnershipTransfer = request.Tokens.TransferNFTOwnership

		c.log.Info("publishNFTEvents: Publishing event",
			"index", i,
			"nftID", nft.NFTId,
			"hasData", nft.Data != "",
			"transactionID", transactionId,
		)

		if err := c.publishNewNftEvent(&event); err != nil {
			c.log.Error("publishNFTEvents: NFT event publish failed",
				"nft", nft.NFTId,
				"err", err,
			)
		} else {
			c.log.Info("publishNFTEvents: Event published successfully",
				"nftID", nft.NFTId,
				"topic", nft.NFTId,
			)
		}
	}
}

// GetChildNFTs returns NFTs whose parent_token_id matches parentNFTId.
// Empty slice when no children exist or the parent is unknown.
func (c *Core) GetChildNFTs(parentNFTId string) ([]types.NFTBalance, error) {
	childRows, err := c.w.GetChildNFTs(parentNFTId)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("GetChildNFTs: query failed", "parentNFTId", parentNFTId, "err", err)
		return []types.NFTBalance{}, fmt.Errorf("failed to get child NFTs, error: %w", err)
	}
	children := make([]types.NFTBalance, 0, len(childRows))
	for _, ch := range childRows {
		children = append(children, types.NFTBalance{
			NFTId:    ch.TokenID,
			NFTValue: ch.TokenValue,
		})
	}
	return children, nil
}

// GetParentNFT returns the parent NFT of childNFTId, or (nil, nil) when the
// child is unknown locally or has no parent.
func (c *Core) GetParentNFT(childNFTId string) (*types.NFTBalance, error) {
	child, err := c.w.GetTokenByTokenID(childNFTId)
	if err != nil {
		return nil, nil
	}
	if !child.ParentTokenID.Valid || child.ParentTokenID.String == "" {
		return nil, nil
	}
	parent, err := c.w.GetTokenByTokenID(child.ParentTokenID.String)
	if err != nil {
		// Parent ID recorded but row not in this wallet — return ID with zero value.
		return &types.NFTBalance{NFTId: child.ParentTokenID.String}, nil
	}
	return &types.NFTBalance{
		NFTId:    parent.TokenID,
		NFTValue: parent.TokenValue,
	}, nil
}

// GetNFTsByDid returns an empty NFT list stub for a given DID.
func (c *Core) GetNFTsByDid(did string) ([]types.NFTBalance, error) {
	nftTokenType := int16(models.GetTokenTypeID(constants.TokenType_NFT))
	// get list of NFTs
	nftInfoList, err := c.w.GetTokenByDIDAndTokenType(did, nftTokenType)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get nfts", "err", err)
		return []types.NFTBalance{}, fmt.Errorf("failed to get nfts, error: %w", err)
	}
	// List out all nft ids and their values, and return the list
	var nftInfo []types.NFTBalance
	for _, nft := range nftInfoList {
		nftInfo = append(nftInfo, types.NFTBalance{
			NFTId:    nft.TokenID,
			NFTValue: nft.TokenValue,
		})
	}
	return nftInfo, nil
}

// CheckNFTFolderExists checks if the NFT folder exists on disk.
// Returns the folder path if it exists, empty string if not.
func (c *Core) CheckNFTFolderExists(nft string) (string, error) {
	dirPath := path.Join(c.nftDir, nft)
	_, err := os.Stat(dirPath)
	if err == nil {
		return dirPath, nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	return "", err
}

// GetAllNFTs returns all NFT tokens from the database.
func (c *Core) GetAllNFTs() ([]models.Token, error) {
	nfts, err := c.w.GetNFTTokens()
	if err != nil {
		return nil, err
	}
	return nfts, nil
}

// GetNFTChain returns the token chain for a given NFT token ID.
func (c *Core) GetNFTChain(nftID string) ([]models.TokenChainResponse, error) {
	nftTokenChain, err := c.w.GetNFTChainByTokenID(nftID)
	if err != nil {
		return nil, err
	}
	return nftTokenChain, nil
}

// DumpTokenChain stubs a token chain dump operation.
func (c *Core) DumpTokenChain(req *model.TCDumpRequest) *model.TCDumpReply {
	return &model.TCDumpReply{}
}
