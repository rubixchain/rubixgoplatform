package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type MerkleRootPayload struct {
	PublisherPeerID string `json:"peer_id"`
	Epoch           string `json:"epoch"`       // hour-based timestamp for grouping
	MerkleRoot      string `json:"merkle_root"` // computed root
	Count           int    `json:"count"`       // number of block hashes included
	Timestamp       int64  `json:"timestamp"`   // unix time when published
}

type GetRemoteChildrenReq struct {
	Epoch string `json:"epoch"`
	Level int    `json:"level"`
	Index int    `json:"index"`
}
type GetRemoteChildrenReply struct {
	Status      bool     `json:"status"`
	Message     string   `json:"message"`
	ChildHashes []string `json:"child_hashes"`
}

// Enhanced subscription setup with error handling
func (c *Core) SubscribeTxnSetup() {
	// Initialize the transaction processor
	c.initDynamicTxnProcessor()

	topic := RubixTxnTopic
	err := c.ps.SubscribeTopic(topic, c.TxnCallBack)
	if err != nil {
		c.log.Error("Unable to subscribe to topic", "topic", topic, "error", err)
		return
	}
	c.log.Info("Successfully subscribed to topic: " + topic)
}

// Enhanced callback with dynamic scaling integration
func (c *Core) TxnCallBack(peerID string, topic string, data []byte) {
	c.log.Debug("piblisher peer id : ", peerID)

	var newEvent model.PubSubTxnInfo
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to parse published event", "error", err, "data", string(data))
		return
	}

	// add publisher to peer did table
	unknownDIDType := -1
	publisherDetails := &wallet.DIDPeerMap{
		DID:     newEvent.PublisherDID,
		PeerID:  peerID,
		DIDType: &unknownDIDType,
	}
	err = c.AddPeerDetails(*publisherDetails)
	if err != nil {
		c.log.Error("failed to add publisher info to DB")
	}

	// Check for duplicate transactions
	if _, exists := c.txnProcessor.processedTxns.LoadOrStore(newEvent.BlockHash, time.Now()); exists {
		c.log.Info("Duplicate transaction ignored", "blockHash", newEvent.BlockHash)
		return
	}

	// INCREMENT COUNTER when new transaction is processed
	atomic.AddInt64(&c.txnProcessor.processedTxnCount, 1)

	c.log.Info("Received transaction", "blockHash", newEvent.BlockHash, "mode", newEvent.AssetType)

	// Update queue length metric for dynamic scaling
	currentQueueLen := int64(len(c.txnProcessor.txnQueue))
	c.txnProcessor.queueLength = currentQueueLen

	// Queue transaction for processing with enhanced timeout handling
	select {
	case c.txnProcessor.txnQueue <- &newEvent:
		c.log.Debug("Transaction queued successfully",
			"blockHash", newEvent.BlockHash,
			"queueLength", currentQueueLen)

	case <-time.After(5 * time.Second):
		c.log.Error("Failed to queue transaction - queue full",
			"blockHash", newEvent.BlockHash,
			"queueLength", len(c.txnProcessor.txnQueue))

		if currentQueueLen > int64(c.txnProcessor.queueThreshold) {
			c.log.Warn("Queue threshold exceeded - scaling may be needed",
				"current", currentQueueLen,
				"threshold", c.txnProcessor.queueThreshold)
		}
		return

	case <-c.txnProcessor.ctx.Done():
		c.log.Info("Transaction processor shutting down")
		return
	}
}

// Process transaction with retry mechanism
func (c *Core) processTxnWithRetry(txnEvent *model.PubSubTxnInfo, workerID int) {
	var lastErr error

	for attempt := 0; attempt < c.txnProcessor.maxRetries; attempt++ {
		if attempt > 0 {
			c.log.Info("Retrying transaction processing",
				"blockHash", txnEvent.BlockHash,
				"attempt", attempt+1,
				"workerID", workerID)
			time.Sleep(c.txnProcessor.retryDelay * time.Duration(attempt))
		}

		err := c.processSingleTransaction(txnEvent)
		if err == nil {
			c.log.Info("Transaction processed successfully",
				"blockHash", txnEvent.BlockHash,
				"workerID", workerID)
			return
		}

		lastErr = err
		c.log.Error("Transaction processing failed",
			"blockHash", txnEvent.BlockHash,
			"attempt", attempt+1,
			"error", err,
			"workerID", workerID)
	}

	// All retries exhausted - handle failure
	c.handleFailedTransaction(txnEvent, lastErr)
}

// Enhanced single transaction processing with better error handling
func (c *Core) processSingleTransaction(newEvent *model.PubSubTxnInfo) error {
	receiverDid := newEvent.ReceiverDID

	// Initialize block with error handling
	txnBlock := block.InitBlock(newEvent.TxnBlock, nil)
	if txnBlock == nil {
		return fmt.Errorf("failed to initialize transaction block for txn %s", newEvent.BlockHash)
	}
	txnBlockType := newEvent.TxnType
	c.log.Debug("^^^^^^^^^^^^^^^txn type : ", txnBlockType, "txn id ", newEvent.TransactionID)
	if txnBlockType == block.TokenTransferredType || txnBlockType == block.TokenDeployedType || txnBlockType == block.TokenExecutedType {
		TransactionIDFromTheTransactionBlock := newEvent.TransactionID

		transaction := model.FullNodeTxnHistoryInfo{
			TransactionID:    TransactionIDFromTheTransactionBlock,
			TransactionValue: newEvent.TransactionValue,
			BlockHash:        newEvent.BlockHash,
		}
		transactionHistErr := c.w.AddTransactionsToFullNodeTransactionHistoryTable(&transaction)
		if transactionHistErr != nil {
			c.log.Error("faile to add transaction details to Fullnode transaction history table for the transaction: ", transaction.TransactionID, "error", transactionHistErr)
		}

	}

	currentOwner := txnBlock.GetOwner()
	tokensList := txnBlock.GetTransTokens()

	if len(tokensList) == 0 {
		return fmt.Errorf("no tokens found in transaction %s", newEvent.BlockHash)
	}

	logmsg := fmt.Sprintf("^^^^^^^^^processing ASSET type : %d ; block height : %d", newEvent.AssetType, newEvent.LatestBlockHeight)
	c.log.Debug(logmsg)
	switch newEvent.AssetType {
	case RBTTokenType, FTTokenType:
		return c.processTransferTransaction(newEvent, txnBlock, tokensList, receiverDid, currentOwner)

	case SmartContractTokenType, NFTTokenType:
		return c.processContractTransaction(newEvent, txnBlock, tokensList[0], currentOwner)

	default:
		return fmt.Errorf("unsupported transaction mode: %v", newEvent.AssetType)
	}
}

// Separate transfer transaction processing
func (c *Core) processTransferTransaction(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokensList []string, receiverDid, currentOwner string) error {
	var errors []string

	for _, tokenId := range tokensList {
		c.log.Debug("...... processing token ", tokenId)
		if err := c.processTransferToken(newEvent, txnBlock, tokenId, receiverDid, currentOwner); err != nil {
			errors = append(errors, fmt.Sprintf("token %s: %v", tokenId, err))
			continue
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("transfer processing errors: %v", errors)
	}

	return nil
}

// Process individual transfer token
func (c *Core) processTransferToken(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, receiverDid, currentOwner string) error {
	// Token generated type handling
	if newEvent.TxnType == block.TokenGeneratedType {
		if newEvent.PublisherDID != "" {
			if currentOwner != newEvent.PublisherDID {
				return fmt.Errorf("publisher DID mismatch for token generation: expected %s, got %s", currentOwner, newEvent.PublisherDID)
			}
		}

		if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
			return fmt.Errorf("failed to add generated block to token chain, err: %v", err)
		}
		if err := c.AddTokenContentToPSQL(tokenId, newEvent.AssetType); err != nil {
			return fmt.Errorf("failed to add token's ipfs content to psql db, err: %v", err)
		}
		// update block height if required
		latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
		if err != nil {
			c.log.Error("failed to get block height")
		}
		newEvent.LatestBlockHeight = latestBlockHeight
		syncStatus := wallet.SyncCompleted
		receivedBlocks := ReceivedBlock{
			GenesisBlock: txnBlock,
			LatestBlock:  txnBlock,
		}
		return c.AddTokenToRespectiveTable(tokenId, currentOwner, receivedBlocks, newEvent, syncStatus)
	}

	// Regular transfer processing with enhanced validation
	return c.processRegularTransfer(newEvent, txnBlock, tokenId, receiverDid, newEvent.FullNodeAsProviderPeerID)
}

// Process regular transfer with enhanced validation
func (c *Core) processRegularTransfer(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, receiverDid string, fullNodePubPeerID string) error {
	// Get current and latest block numbers
	currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get current block number: %v", err)
	}
	c.log.Debug("current block number is ", currentBlockNumber)
	txnBlockType := txnBlock.GetTransType()

	if currentBlockNumber != 0 {
		tokenType := txnBlock.GetTokenType(tokenId)
		latestTokenBlock := c.w.GetFullNodeLatestTokenBlock(tokenId, tokenType)
		if latestTokenBlock == nil {
			//connect to publisher and fetch complete token chain
			p, err := c.getPeer(newEvent.PublisherDID)
			if err != nil {
				c.log.Error("failed to sync full token chain, failed to open peer connection with publisher ", newEvent.PublisherDID)
				return fmt.Errorf("failed to open peer connection with publisher ", newEvent.PublisherDID)
			}
			defer p.Close()
			tokenSyncInfo := &TokenSyncInfo{
				TokenID:   tokenId,
				TokenType: tokenType,
				AssetType: newEvent.AssetType,
			}

			//If fullnode has to sync it from other fullnode instead of a normal publisher node,
			// Sync From FullNode has to be true
			if fullNodePubPeerID != "" {
				tokenSyncInfo.SyncFromFullnode = true
			}
			err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
			if err != nil {
				c.log.Error("failed to sync token chain for token ", tokenId, "error", err)
				return fmt.Errorf("failed to get latest block for token %s - may need sync", tokenId)

			}
			c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
			return nil
		}

		// check if token exists in postgres table, add if doesn't
		err := c.ReadTokenContentFromPSQL(tokenId, newEvent.AssetType)
		if err != nil {
			if err := c.AddTokenContentToPSQL(tokenId, newEvent.AssetType); err != nil {
				c.log.Error("failed to add token's ipfs content to psql db, err: %v", err)
			}
		}

		latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
		if err != nil {
			return fmt.Errorf("failed to get latest block number: %v", err)
		}

		c.log.Debug("latest block number is ", latestBlockNumber)

		// Check for missing blocks
		if latestBlockNumber+1 != currentBlockNumber {
			if latestBlockNumber == currentBlockNumber {
				latestBlockHash, _ := latestTokenBlock.GetHash()
				if newEvent.BlockHash == latestBlockHash {
					c.log.Debug("fullnode is updated with complete token chain of token ", tokenId)
					return nil
				}
				return fmt.Errorf("invalid blocks detected: latest block number=%d, current block number=%d", latestBlockNumber, currentBlockNumber)
			} else if latestBlockNumber < currentBlockNumber {
				//connect to publisher and fetch complete token chain
				var p *ipfsport.Peer
				//If fullnode has to sync it from other fullnode instead of a normal publisher node,
				//It has to connect with FullNode using their PeerID, because they don't have DID
				if fullNodePubPeerID != "" {
					p, err = c.pm.OpenPeerConn(fullNodePubPeerID, "", c.getCoreAppName(fullNodePubPeerID))
					if err != nil {
						c.log.Error("Failed to get peer connection", "err", err)
						return err
					}
				} else {
					p, err = c.getPeer(newEvent.PublisherDID)
					if err != nil {
						c.log.Error("failed to sync full token chain, failed to open peer connection with publisher ", newEvent.PublisherDID)
						return fmt.Errorf("failed to open peer connection with publisher ", newEvent.PublisherDID)
					}
				}
				defer p.Close()
				tokenSyncInfo := &TokenSyncInfo{
					TokenID:   tokenId,
					TokenType: tokenType,
					AssetType: newEvent.AssetType,
				}
				if fullNodePubPeerID != "" {
					tokenSyncInfo.SyncFromFullnode = true
				}
				err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
				if err != nil {
					c.log.Error("failed to sync token chain for token ", tokenId, "error", err)
					return fmt.Errorf("failed to get latest block for token %s - may need sync", tokenId)

				}
				c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
				return nil
			}
			return fmt.Errorf("mismatch of blocks detected: latest block number=%d, current block number=%d", latestBlockNumber, currentBlockNumber)
		}

		// Validate ownership
		previousOwner := latestTokenBlock.GetOwner()
		currentOwner := txnBlock.GetOwner()
		if txnBlockType == block.TokenBurntType || txnBlockType == block.TokenIsBurntForFT {
			if currentOwner != newEvent.PublisherDID {
				errMsg := fmt.Sprintf("publisher DID mismatch with current owner in burnt block of token : %v, expected %s, got %s", tokenId, currentOwner, newEvent.PublisherDID)
				c.log.Error(errMsg)
				doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
					TokenID:        tokenId,
					AssetType:      newEvent.AssetType,
					TokenType:      tokenType,
					PublisherDID:   newEvent.PublisherDID,
					ClaimedOwnerI:  previousOwner,
					ClaimedOwnerII: newEvent.PublisherDID,
					ErrorMessage:   "publisher DID mismatch with current owner in burnt block",
				}
				err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
				if err != nil {
					errMsg = errMsg + "failed to update double spent token in tables"
					return fmt.Errorf("%v", errMsg)
				}
				c.log.Info("updated double spent token in tables : ", tokenId)

				return nil
			}
		}
		if previousOwner != newEvent.PublisherDID {
			errMsg := fmt.Sprintf("publisher DID mismatch with prev-owner for token: %v, expected %s, got %s; ", previousOwner, newEvent.PublisherDID)
			c.log.Error(errMsg)
			// since we hace ensured above that fullnode does not have any missing blocks,
			// so now if publisher is not the previous owner, we can safely assume that it is a double spent token
			doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
				TokenID:        tokenId,
				AssetType:      newEvent.AssetType,
				TokenType:      tokenType,
				PublisherDID:   newEvent.PublisherDID,
				ClaimedOwnerI:  previousOwner,
				ClaimedOwnerII: newEvent.PublisherDID,
				ErrorMessage:   "publisher DID mismatch with prev-owner",
			}
			err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
			if err != nil {
				errMsg = errMsg + "failed to update double spent token in tables"
				return fmt.Errorf("%v", errMsg)
			}
			c.log.Info("updated double spent token in tables : ", tokenId)

			return nil
		}

		// Validate receiver for transfers
		if receiverDid != "" {
			if currentOwner != receiverDid {
				return fmt.Errorf("receiver DID mismatch: expected %s, got %s", receiverDid, currentOwner)
			}
		}
	}

	receivedBlock := ReceivedBlock{
		LatestBlock: txnBlock,
	}
	// if it is a genesis block, then fetch token's ipfs content and store in psql db
	if currentBlockNumber == 0 {
		if err := c.AddTokenContentToPSQL(tokenId, newEvent.AssetType); err != nil {
			return fmt.Errorf("failed to add token's ipfs content to psql db, err: %v", err)
		}
		receivedBlock.GenesisBlock = txnBlock
	}

	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add block to token chain: %v", err)
	}
	// update block height if required
	latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		c.log.Error("failed to get block height")
	}
	newEvent.LatestBlockHeight = latestBlockHeight
	syncStatus := wallet.SyncCompleted
	// Add to database and blockchain

	if err := c.AddTokenToRespectiveTable(tokenId, receiverDid, receivedBlock, newEvent, syncStatus); err != nil {
		return fmt.Errorf("failed to add token to table: %v", err)
	}

	c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
	return nil
}

// Process contract-related transactions (Smart Contract and NFT operations)
func (c *Core) processContractTransaction(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, currentOwner string) error {
	// Handle token generated type (new deployments)
	if newEvent.TxnType == block.TokenGeneratedType || newEvent.TxnType == block.TokenDeployedType {
		if currentOwner != newEvent.PublisherDID {
			return fmt.Errorf("publisher DID mismatch for contract deployment: expected %s, got %s", currentOwner, newEvent.PublisherDID)
		}

		// Add block directly to token chain for new deployments
		if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
			return fmt.Errorf("failed to add contract block to token chain: %v", err)
		}

		// if it is a genesis block, then fetch token's ipfs content and store in psql db
		if err := c.AddTokenContentToPSQL(tokenId, newEvent.AssetType); err != nil {
			return fmt.Errorf("failed to add token's ipfs content to psql db, err: %v", err)
		}

		// update block height if required
		latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
		if err != nil {
			c.log.Error("failed to get block height")
		}
		newEvent.LatestBlockHeight = latestBlockHeight
		syncStatus := wallet.SyncCompleted
		receivedBlock := ReceivedBlock{
			LatestBlock:  txnBlock,
			GenesisBlock: txnBlock,
		}
		if err := c.AddTokenToRespectiveTable(tokenId, currentOwner, receivedBlock, newEvent, syncStatus); err != nil {
			return fmt.Errorf("failed to add contract token to table: %v", err)
		}
		c.log.Info("New contract deployment processed", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
		return nil
	}

	// Handle existing contract executions with validation
	return c.processContractExecution(newEvent, txnBlock, tokenId)
}

// Process execution of existing contracts with block validation
func (c *Core) processContractExecution(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId string) error {
	// Get token type and latest block for validation
	tokenType := txnBlock.GetTokenType(tokenId)
	latestTokenBlock := c.w.GetFullNodeLatestTokenBlock(tokenId, tokenType)
	if latestTokenBlock == nil {
		//connect to publisher and fetch complete token chain
		p, err := c.getPeer(newEvent.PublisherDID)
		if err != nil {
			c.log.Error("failed to sync full sc chain, failed to open peer connection with publisher ", newEvent.PublisherDID)
			return fmt.Errorf("failed to open peer connection with publisher - %v ", newEvent.PublisherDID)
		}
		defer p.Close()
		tokenSyncInfo := &TokenSyncInfo{
			TokenID:   tokenId,
			TokenType: tokenType,
			AssetType: newEvent.AssetType,
		}
		err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
		if err != nil {
			c.log.Error("failed to sync sc chain for token ", tokenId, "error", err)
			return fmt.Errorf("failed to get latest block for sc %s - may need sync", tokenId)

		}
		c.log.Info("execution block processed successfully", "sc hash ", tokenId, "blockHash ", newEvent.BlockHash)
		return nil
	}

	// Validate block sequence continuity
	latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get latest block number for contract: %v", err)
	}

	currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get current block number for contract: %v", err)
	}

	// Check for missing blocks in contract chain
	if latestBlockNumber+1 != currentBlockNumber {
		return fmt.Errorf("missing blocks in contract chain %s: latest=%d, current=%d",
			tokenId, latestBlockNumber, currentBlockNumber)
	}

	// Validate publisher ownership
	previousOwner := latestTokenBlock.GetOwner()

	// deployer should be the contract owner for all smart contracts, but for NFTs, owners keep changing
	if newEvent.AssetType == NFTTokenType {
		if previousOwner != newEvent.PublisherDID {
			return fmt.Errorf("NFT publisher DID mismatch: expected %s, got %s", previousOwner, newEvent.PublisherDID)
		}
	}

	// Add validated block to contract chain
	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add contract execution block to chain: %v", err)
	}
	currentOwner := txnBlock.GetOwner()
	// update block height if required
	latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		c.log.Error("failed to get block height")
	}
	newEvent.LatestBlockHeight = latestBlockHeight
	syncStatus := wallet.SyncCompleted
	receivedBlock := ReceivedBlock{
		LatestBlock: txnBlock,
	}
	if err := c.AddTokenToRespectiveTable(tokenId, currentOwner, receivedBlock, newEvent, syncStatus); err != nil {
		return fmt.Errorf("failed to add contract token to table: %v", err)
	}

	c.log.Info("Contract execution processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
	return nil
}

// Handle failed transactions after all retries
func (c *Core) handleFailedTransaction(txnEvent *model.PubSubTxnInfo, lastErr error) {
	c.log.Error("Transaction processing failed permanently",
		"blockHash", txnEvent.BlockHash,
		"error", lastErr)

	// Implement failure handling strategy:
	// 1. Store in failed transactions table for manual review
	// 2. Trigger alerts
	// 3. Attempt alternative processing paths

	// Example: Store in failed transactions table
	failedTxn := &model.FailedTransaction{
		BlockHash:    txnEvent.BlockHash,
		PublisherDID: txnEvent.PublisherDID,
		Error:        lastErr.Error(),
		FailedAt:     time.Now(),
		RetryCount:   c.txnProcessor.maxRetries,
	}

	if err := c.w.StoreFailedTransaction(failedTxn); err != nil {
		c.log.Error("Failed to store failed transaction", "blockHash", txnEvent.BlockHash, "error", err)
	}
}

// Graceful shutdown
func (c *Core) ShutdownTxnProcessor() {
	if c.txnProcessor != nil {
		c.log.Info("Shutting down transaction processor")
		c.txnProcessor.cancel()

		// Close the queue channel to signal workers to finish current work
		close(c.txnProcessor.txnQueue)

		// Wait for all workers to complete with timeout
		done := make(chan struct{})
		go func() {
			c.txnProcessor.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			c.log.Info("All transaction workers shut down gracefully")
		case <-time.After(30 * time.Second):
			c.log.Warn("Transaction workers shutdown timeout - forcing termination")
		}
	}
}

func (c *Core) RetryFailedTOSyncTokens() error {
	failedToSyncTokens, err := c.w.GetAllFailedToSyncTokens()
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			c.log.Info("There are no failed tokens to sync")
			return nil
		} else {
			c.log.Error("failed to get tokens which are failed to sync ", "error", err)
			return err
		}

	}
	if failedToSyncTokens == nil {
		c.log.Info("There are NO failed to sync tokens which needs retry of syncing")
	} else {
		for _, failedToSyncToken := range failedToSyncTokens {
			//call synctokenchain api to sync each token
			//connect to publisher and fetch complete token chain
			p, err := c.getPeer(failedToSyncToken.Did)
			if err != nil {
				c.log.Error("failed to sync full token chain, failed to open peer connection with publisher ", failedToSyncToken.Did, "error: ", err)
				return fmt.Errorf("failed to open peer connection with publisher %v, error: %v", failedToSyncToken.Did, err)
			}
			defer p.Close()
			tokenSyncInfo := &TokenSyncInfo{
				TokenID:   failedToSyncToken.TokenID,
				TokenType: failedToSyncToken.TokenType,
				AssetType: failedToSyncToken.AssetType,
			}
			err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
			if err != nil {
				c.log.Error("failed to sync token chain for token ", failedToSyncToken.TokenID, "error", err)
				return fmt.Errorf("failed to get latest block for token %s - may need sync", failedToSyncToken.TokenID)

			}
		}
	}

	return nil
}

// ---- Domain separated Hashing ----

// Leaf: hash with prefix 0x00
func hashLeaf(data []byte) []byte {
	h := sha256.New()
	h.Write([]byte{0x00})
	h.Write(data)
	return h.Sum(nil)
}

// Internal node: hash with prefix 0x01 and up to 4 children
func hashNode(children ...[]byte) []byte {
	h := sha256.New()
	h.Write([]byte{0x01})
	for _, c := range children {
		h.Write(c)
	}
	return h.Sum(nil)
}

// EMPTY value for padding children
var EMPTY_HASH = hashLeaf(nil)

// ---- Arity-4 Merkle Root Calculation ----

func ComputeArity4MerkleRoot(blockHashes []string) (string, error) {

	// Convert hex → raw bytes and normalize input
	leaves := make([][]byte, 0, len(blockHashes))
	for _, bh := range blockHashes {
		b, err := hex.DecodeString(bh)
		if err != nil {
			return "", err
		}
		leaves = append(leaves, hashLeaf(b))
	}

	// If no blocks → return empty leaf hash
	if len(leaves) == 0 {
		return hex.EncodeToString(EMPTY_HASH), nil
	}

	// Sort lexicographically (VERY IMPORTANT for determinism)
	sort.Slice(leaves, func(i, j int) bool {
		return bytes.Compare(leaves[i], leaves[j]) < 0
	})

	// Build tree bottom-up in arity-4 layers
	currentLevel := leaves

	for len(currentLevel) > 1 {
		nextLevel := [][]byte{}

		for i := 0; i < len(currentLevel); i += 4 {
			// Collect up to 4 children
			c1 := currentLevel[i]

			var c2, c3, c4 []byte = EMPTY_HASH, EMPTY_HASH, EMPTY_HASH

			if i+1 < len(currentLevel) {
				c2 = currentLevel[i+1]
			}
			if i+2 < len(currentLevel) {
				c3 = currentLevel[i+2]
			}
			if i+3 < len(currentLevel) {
				c4 = currentLevel[i+3]
			}

			parent := hashNode(c1, c2, c3, c4)
			nextLevel = append(nextLevel, parent)
		}

		currentLevel = nextLevel
	}

	// Final element is the Merkle Root
	return hex.EncodeToString(currentLevel[0]), nil
}

// This will compute the merkle root of all the block_hashes which it received in the last one hour
func (c *Core) ComputeLatestMerkleRoot() (string, error) {
	prevEpoch := time.Now().Add(-26 * time.Hour).Format("2006-01-02T15") //we need last one hour's block_hashes, so we are passing the previous hour's epoch
	records, err := c.w.ReadBlocksForEpoch(prevEpoch)
	if err != nil {
		return "", err
	}
	blockHashes := make([]string, len(records))
	for i, record := range records {
		blockHashes[i] = record.BlockHash
	}

	return ComputeArity4MerkleRoot(blockHashes)

}

func (c *Core) PublishMerkleRoot() error {

	// Step 1: Fetch previous 1hour blockhash entries
	prevEpoch := time.Now().Add(-26 * time.Hour).Format("2006-01-02T15") //we need last one hour's block_hashes, so we are passing the previous hour's epoch
	records, err := c.w.ReadBlocksForEpoch(prevEpoch)
	if err != nil {
		c.log.Error("Failed to read blocks for Merkle computation", "err", err)
		return err
	}
	c.log.Debug("no.of records read from FullnodeBlockHashTable", len(records))

	// Step 2: Extract blockhash strings
	blockHashes := make([]string, len(records))
	for i, r := range records {
		blockHashes[i] = r.BlockHash
	}

	c.log.Debug("Number of blockhashes used to compute merkle root", len(blockHashes))
	// Step 3: Compute Merkle Root
	root, err := ComputeArity4MerkleRoot(blockHashes)
	if err != nil {
		c.log.Error("Failed computing Merkle root", "err", err)
		return err
	}
	// c.log.Debug("**computed merkle root is: ", root)

	// Step 4: Prepare payload
	payload := MerkleRootPayload{
		PublisherPeerID: c.peerID,
		Epoch:           prevEpoch, //Epoch for which we have computed the merkle root
		MerkleRoot:      root,
		Count:           len(blockHashes),
		Timestamp:       time.Now().Unix(),
	}

	c.log.Info("Publishing Merkle root", "root", root, "count", len(blockHashes), "for the epoch", payload.Epoch)

	// Step 5: Publish to pubsub topic
	return c.ps.Publish("merkle_root", payload)
}

// StartMerkleRootSubscriber subscribes to the "merkle_root" pubsub topic
// and forwards decoded messages to handleMerkleRootMessage.
func (c *Core) StartMerkleRootSubscriber() error {
	return c.ps.SubscribeTopic("merkle_root", func(fromPeerID string, topic string, data []byte) {
		var payload MerkleRootPayload

		if err := json.Unmarshal(data, &payload); err != nil {
			c.log.Error("Failed to unmarshal merkle_root payload", "err", err)
			return
		}

		// If PublisherPeerID is not set in payload, fall back to pubsub sender
		if payload.PublisherPeerID == "" {
			payload.PublisherPeerID = fromPeerID
		}

		// Ignore messages published by myself (optional but usually desired)
		if payload.PublisherPeerID == c.peerID {
			return
		}

		c.handleMerkleRootMessage(payload)
	})
}

func (c *Core) handleMerkleRootMessage(remote MerkleRootPayload) {
	// Ignore if message from me
	if remote.PublisherPeerID == c.peerID {
		return
	}
	debugMsg := fmt.Sprintf("remote epoch %s,  for which the root is %s,", remote.MerkleRoot, remote.Epoch)
	c.log.Debug(debugMsg)
	// Ensure epoch matches my working window
	myEpoch := time.Now().Add(-26 * time.Hour).Format("2006-01-02T15")
	if remote.Epoch != myEpoch {
		c.log.Info("Ignoring Merkle root for different epoch", "remote", remote.Epoch, "local", myEpoch)
		return
	}

	myRoot, err := c.ComputeLatestMerkleRoot()
	if err != nil {
		c.log.Error("Failed computing local Merkle root", "err", err)
		return
	}

	if myRoot == remote.MerkleRoot {
		c.log.Info("Merkle matched. Already in sync with peer", "peer", remote.PublisherPeerID)
		return
	}

	c.log.Warn("Merkle mismatch — starting reconciliation from the peer: ", remote.PublisherPeerID, "myroot", myRoot, "RemoteRoot", remote.MerkleRoot)

	//finding the missing block_hashes when compared to the remote publisher
	missing := c.reconcileWithPeer(remote)

	c.log.Debug("***no of missing blocks are: ******", len(missing))

	if len(missing) > 0 {
		c.log.Warn("Missing hashes discovered", "count", len(missing))
		err := c.fetchBlocks(remote.PublisherPeerID, missing)
		if err != nil {
			c.log.Error("failed to fetch missing blocks the peer: ", remote.PublisherPeerID)
		}
	}
}

// If remoteRoot doesn't match with localRoot for a particular hour the following function will get called
func (c *Core) reconcileWithPeer(payload MerkleRootPayload) []string {
	c.log.Debug("***reconcileWithPeer function got called***")
	missing := []string{}
	compareQueue := []struct {
		level int
		index int
	}{{0, 0}} // start at root

	// remoteRoot, err := c.remoteGetNode(payload.PublisherPeerID, , 0, 0)
	// if err != nil {
	// 	return missing
	// }
	remoteRoot := payload.MerkleRoot

	c.log.Debug("**remoteRoot in reconcileWithPeer:  *****", remoteRoot)

	// Compute local tree leaf + internal structure in memory ONCE
	localTree := c.w.BuildMerkleStructForComparison(payload.Epoch)
	if len(localTree.Levels) == 0 {
		c.log.Debug("**not able to compute local merkle tree**")
	}

	if localTree.Root() == remoteRoot {
		c.log.Debug("**No need for reconcile, local and remote roots are matching***")
		return missing
	}

	c.log.Debug("length of the compareQue is: ", len(compareQueue))
	// BFS tree walk
	for len(compareQueue) > 0 {
		item := compareQueue[0]
		compareQueue = compareQueue[1:]

		localChildren := localTree.GetChildren(item.level, item.index)
		c.log.Debug("*length of the localchildren: ****", len(localChildren), "localchildren: ", localChildren)

		c.log.Debug("comparison level: ", item.level, "comparison index", item.index, "epoch", payload.Epoch)
		remoteChildren, err := c.getRemoteChildBlockHashes(payload.PublisherPeerID, payload.Epoch, item.level, item.index)
		if err != nil {
			c.log.Debug("failed to get remote fullnodes child block hashes, err: ", err)
			continue
		}
		c.log.Debug("lenth of the remote children are: ", len(remoteChildren))

		// compare children in arity-4 manner
		for i := 0; i < 4; i++ {
			if remoteChildren[i] == "" {
				debugMsg := fmt.Sprintf("%dth blash hash is empty", i)
				c.log.Debug(debugMsg)
				continue
			}

			if localChildren[i] == remoteChildren[i] {
				debugMsg := fmt.Sprintf("remote and local %d th blackHashes are same, block_hash is: %s ", i, localChildren[i])
				c.log.Debug(debugMsg)
				continue
			}

			// If level is leaf level → collect missing
			if localTree.IsLeaf(item.level + 1) {
				debugMsg := fmt.Sprintf("%dth remote children: %s, local children: %s", i, remoteChildren[i], localChildren[i])
				c.log.Debug(debugMsg)
				c.log.Debug("missing block hash is: ", remoteChildren[i])
				missing = append(missing, remoteChildren[i])
			} else {
				compareQueue = append(compareQueue, struct{ level, index int }{item.level + 1, item.index*4 + i})
				c.log.Debug("length of the compareQue is: ", len(compareQueue))
			}
		}
	}

	return missing
}

//Using this function, one fullnode can request a block from other fullnode, after getting the block it will add the details in sqlite tables also
func (c *Core) requestBlockFromPeer(peerID string, blockHash string) error {
	debugMsg := fmt.Sprintf("**requestBlockFromPeer function got called, fetching the block with hash %s***", blockHash)
	c.log.Debug(debugMsg)
	p, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		c.log.Error("Failed to get peer connection", "err", err)
		return err
	}
	req := SyncTokenBlockFromFullNodeRequest{
		BlockHash: blockHash,
	}
	var resp SyncTokenBlockReply
	err1 := p.SendJSONRequest("POST", APISyncTokenBlockFromFullNode, nil, &req, &resp, false)
	if err1 != nil {
		c.log.Error("failed to get token block from other remote fullnode, error: ", err1)
		return err1
	}
	if !resp.Status {
		c.log.Error("failed to get token block from other remote fullnode, msg: ", resp.Message)
		return fmt.Errorf(resp.Message)
	}
	if strings.Contains(resp.Message, "Sent requested block sucessfully") {
		//TODO: add received block to the tokenchin of all those tokens which are there in that block
		blockBytes := resp.SyncTCBlock
		c.log.Debug("**len of the block received from the remote fullnode***", len(resp.SyncTCBlock))
		// Initialize block with error handling
		blockMap := block.InitBlock(blockBytes, nil)
		if blockMap == nil {
			c.log.Error("failed to initialize the block in requestBlockFromPeer function")
		}
		transactionID := blockMap.GetTid()
		transactionType := blockMap.GetTransType()
		blockPublisherDID := resp.BlockPublisherDID //Ingeneral transaction senderDID
		receiverDID := blockMap.GetReceiverDID()
		currentOwner := blockMap.GetOwner()
		assetType := resp.AssetType

		tokens := blockMap.GetTransTokens()
		if len(tokens) > 0 {
			for _, token := range tokens {
				event := model.PubSubTxnInfo{
					BlockHash:                blockHash,
					TransactionID:            transactionID,
					TxnType:                  transactionType,
					AssetType:                assetType,
					PublisherDID:             blockPublisherDID,
					ReceiverDID:              receiverDID,
					TxnBlock:                 blockBytes,
					LatestBlockHeight:        resp.TokenDetails[token].LatestBlockHeight,
					TokenValue:               resp.TokenDetails[token].TokenValue,
					FullNodeAsProviderPeerID: peerID,
					CreatorDID:               resp.TokenDetails[token].CreatorDID,
				}
				switch event.AssetType {
				case RBTTokenType, FTTokenType:
					err := c.processTransferToken(&event, blockMap, token, receiverDID, currentOwner)
					if err != nil {
						c.log.Error("failed to process transfer token: ", token, "error", err)
						continue

					}
				case NFTTokenType, SmartContractTokenType:
					err := c.processContractTransaction(&event, blockMap, token, currentOwner)
					if err != nil {
						c.log.Error("failed to process contract transaction: ", token, "error", err)

					}
				}

			}
		}

	}

	return nil

}

func (c *Core) fetchBlocks(peerID string, leafHashes []string) error {
	debugMsg := fmt.Sprintf("**fetchBlocks function called, total no.of blocks to fetch are: %d****", len(leafHashes))
	c.log.Debug(debugMsg)
	for _, h := range leafHashes {
		// call existing block request API from peer
		err := c.requestBlockFromPeer(peerID, h)
		if err != nil {
			c.log.Error("Failed fetching block", "hash", h, "from", peerID, "err", err)
			continue
		}
	}
	return nil
}

// This function, connects with the remote peer and gets the child block hashes for given level and index of the merkle tree
func (c *Core) getRemoteChildBlockHashes(peerID, epoch string, level, index int) ([]string, error) {
	c.log.Debug("***getRemoteChildBlockHashes function got called******")

	p, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		c.log.Error("Failed to get peer connection", "err", err)
		return nil, err
	}
	req := GetRemoteChildrenReq{
		Epoch: epoch,
		Level: level,
		Index: index,
	}
	var rep GetRemoteChildrenReply
	err = p.SendJSONRequest("GET", APIGetAllRemoteChildrenBlockHashes, nil, &req, &rep, false)
	//c.log.Debug("syncTokenChainFrom: Sent sync request", "request", syncReq)
	if err != nil {
		c.log.Error("Failed to get remote children block hashes", "err", err)
		return nil, err
	}
	c.log.Debug("just before returning from the getRemoteChildBlockHashes function, childblockhashes are ", rep.ChildHashes)

	return rep.ChildHashes, nil
}

func (c *Core) GetRemoteChildrenBlockHashes(req *ensweb.Request) *ensweb.Result {
	c.log.Debug("***GetRemoteChildrenBlockHashes API handler function got called***")
	var request GetRemoteChildrenReq

	// Parse request
	if err := c.l.ParseJSON(req, &request); err != nil {
		c.log.Warn("Failed to parse request", "error", err)
		return c.l.RenderJSON(req, &GetRemoteChildrenReply{
			Status:  false,
			Message: "Failed to parse request",
		}, http.StatusBadRequest)
	}
	var response GetRemoteChildrenReply
	localTree := c.w.BuildMerkleStructForComparison(request.Epoch)
	localChildren := localTree.GetChildren(request.Level, request.Index)
	c.log.Debug("Length of the local children at handler function side", len(localChildren), "for givenen level ", request.Level, "for index", request.Index, "local children are", localChildren)
	response.Message = "sucessfully sent the child hashes"
	return c.l.RenderJSON(req, &GetRemoteChildrenReply{
		Status:      true,
		Message:     response.Message,
		ChildHashes: localChildren,
	}, http.StatusOK)

}

// This function process incoming transaction history details and add it to Fullnode transaction history table
func (c *Core) processIncomingTransactionHistory(txns []model.FullNodeTxnHistoryInfo) {
	for _, t := range txns {
		err := c.w.AddTransactionsToFullNodeTransactionHistoryTable(&t)
		if err != nil {
			c.log.Error("Failed to store txn history", "txn", t.TransactionID, "err", err)
		}
		// ensuring correct txn amount is stored with fullnode
		storedTxn, err := c.w.ReadFullNodeTransactionHistoryTable(t.TransactionID)
		if err == nil && storedTxn.TransactionValue != t.TransactionValue {
			err = c.w.UpdateFullNodeTransactionHistoryTable(&t)
			if err != nil {
				errMsg := fmt.Sprintf("failed to update transaction amount for transaction id : %v, stored value is : %v and received value is : %v", t.TransactionID, storedTxn.TransactionValue, t.TransactionValue)
				c.log.Error(errMsg)

			}
		}
	}

	c.log.Info("Stored transaction history batch", "count", len(txns))
}

// func (c *Core) ProcessReceivedBlockDetailsFromOtherFullNode(blockBytes []byte) error {
// 	blockMap := block.InitBlock(blockBytes, nil)
// 	if blockMap == nil {
// 		errMsg := fmt.Sprintf("Failed to initialize the block which it received from the remote FullNode")
// 		return fmt.Errorf(errMsg)
// 	}
// 	blockHash, err := blockMap.GetHash()
// 	if err != nil {
// 		c.log.Error("failed to get blockHash, error: ", err)
// 	}
// 	tokensList := blockMap.GetTransTokens()
// 	if len(tokensList) == 0 {
// 		return fmt.Errorf("no tokens found in the block %s", blockHash)
// 	}

// 	transactionID := blockMap.GetTid()
// 	transactionType := blockMap.GetTransType()
// 	for _, tokenID := range tokensList {
// 		err := c.w.AddFullNodeTokenBlock(tokenID, blockMap)
// 		if err != nil {

// 		}
// 	}
// 	return nil
// }
