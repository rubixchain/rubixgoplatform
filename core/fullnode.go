package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// TxnProcessor handles concurrent transaction processing
type TxnProcessor struct {
	txnQueue      chan *model.PubSubTxnInfo
	workerPool    chan struct{}
	processedTxns sync.Map // Track processed transaction IDs
	maxRetries    int
	retryDelay    time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// Initialize the transaction processor
func (c *Core) initTxnProcessor() {
	ctx, cancel := context.WithCancel(context.Background())

	c.txnProcessor = &TxnProcessor{
		txnQueue:   make(chan *model.PubSubTxnInfo, 1000), // Buffered channel for queue
		workerPool: make(chan struct{}, 10),               // Limit concurrent workers
		maxRetries: 3,
		retryDelay: time.Second * 2,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start worker goroutines
	for i := 0; i < 10; i++ {
		c.txnProcessor.wg.Add(1)
		go c.txnWorker(i)
	}
}

// Enhanced subscription setup with error handling
func (c *Core) SubscribeTxnSetup() {
	// Initialize the transaction processor
	c.initTxnProcessor()

	topic := RubixTxnTopic
	err := c.ps.SubscribeTopic(topic, c.TxnCallBack)
	if err != nil {
		c.log.Error("Unable to subscribe to topic", "topic", topic, "error", err)
		return
	}
	c.log.Info("Successfully subscribed to topic: " + topic)
}

// Enhanced callback that queues transactions for processing
func (c *Core) TxnCallBack(peerID string, topic string, data []byte) {

	c.log.Debug("piblisher peer id : ", peerID)

	var newEvent model.PubSubTxnInfo
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to parse published event", "error", err, "data", string(data))
		return
	}

	// add publisher to peer did table
	unknownDIDType := -1 //DID Type -1 means, we don't know  the publisher's DID Type
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

	c.log.Info("Received transaction", "blockHash", newEvent.BlockHash, "mode", newEvent.AssetType)

	// Queue transaction for processing with timeout
	select {
	case c.txnProcessor.txnQueue <- &newEvent:
		c.log.Debug("Transaction queued successfully", "blockHash", newEvent.BlockHash)
	case <-time.After(5 * time.Second):
		c.log.Error("Failed to queue transaction - queue full", "blockHash", newEvent.BlockHash)
		// Optionally implement overflow handling here
	case <-c.txnProcessor.ctx.Done():
		c.log.Info("Transaction processor shutting down")
		return
	}
}

// Worker goroutine for processing transactions
func (c *Core) txnWorker(workerID int) {
	defer c.txnProcessor.wg.Done()

	for {
		select {
		case txnEvent := <-c.txnProcessor.txnQueue:
			c.log.Debug("Worker processing transaction", "workerID", workerID, "blockHash", txnEvent.BlockHash)

			// Acquire worker slot
			c.txnProcessor.workerPool <- struct{}{}

			// Process transaction with retry logic
			c.processTxnWithRetry(txnEvent, workerID)

			// Release worker slot
			<-c.txnProcessor.workerPool

		case <-c.txnProcessor.ctx.Done():
			c.log.Info("Worker shutting down", "workerID", workerID)
			return
		}
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

	currentOwner := txnBlock.GetOwner()
	tokensList := txnBlock.GetTransTokens()

	if len(tokensList) == 0 {
		return fmt.Errorf("no tokens found in transaction %s", newEvent.BlockHash)
	}

	switch newEvent.AssetType {
	case RBTTokenType, FTTokenType:
		return c.processTransferTransaction(newEvent, txnBlock, tokensList, receiverDid, currentOwner)

	case SmartContractTokenType, NFTTokenType:
		return c.processContractTransaction(newEvent, txnBlock, tokensList[0], currentOwner)

	default:
		return fmt.Errorf("unsupported transaction mode: %s", newEvent.AssetType)
	}
}

// Separate transfer transaction processing
func (c *Core) processTransferTransaction(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokensList []string, receiverDid, currentOwner string) error {
	var errors []string

	for _, tokenId := range tokensList {
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
		if currentOwner != newEvent.PublisherDID {
			return fmt.Errorf("publisher DID mismatch for token generation: expected %s, got %s", currentOwner, newEvent.PublisherDID)
		}

		if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
			return fmt.Errorf("failed to add generated block to token chain: %v", err)
		}
		syncStatus := wallet.SyncCompleted

		return c.AddTokenToRespectiveTable(tokenId, currentOwner, txnBlock, newEvent, syncStatus)
	}

	// Regular transfer processing with enhanced validation
	return c.processRegularTransfer(newEvent, txnBlock, tokenId, receiverDid)
}

// Process regular transfer with enhanced validation
func (c *Core) processRegularTransfer(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, receiverDid string) error {
	// Get current and latest block numbers
	currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get current block number: %v", err)
	}

	if currentBlockNumber != 0 {
		tokenType := txnBlock.GetTokenType(tokenId)
		latestTokenBlock := c.w.GetFullNodeLatestTokenBlock(tokenId, tokenType)
		if latestTokenBlock == nil {
			return fmt.Errorf("failed to get latest block for token %s - may need sync", tokenId)
		}

		latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
		if err != nil {
			return fmt.Errorf("failed to get latest block number: %v", err)
		}

		// Check for missing blocks
		if latestBlockNumber+1 != currentBlockNumber {
			return fmt.Errorf("missing blocks detected: latest=%d, current=%d", latestBlockNumber, currentBlockNumber)
		}

		// Validate ownership
		previousOwner := latestTokenBlock.GetOwner()
		if previousOwner != newEvent.PublisherDID {
			return fmt.Errorf("publisher DID mismatch: expected %s, got %s", previousOwner, newEvent.PublisherDID)
		}

		// Validate receiver for transfers
		if receiverDid != "" {
			currentOwner := txnBlock.GetOwner()
			if currentOwner != receiverDid {
				return fmt.Errorf("receiver DID mismatch: expected %s, got %s", receiverDid, currentOwner)
			}
		}
	}

	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add block to token chain: %v", err)
	}
	syncStatus := wallet.SyncCompleted
	// Add to database and blockchain
	if err := c.AddTokenToRespectiveTable(tokenId, receiverDid, txnBlock, newEvent, syncStatus); err != nil {
		return fmt.Errorf("failed to add token to table: %v", err)
	}

	c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
	return nil
}

// Process contract-related transactions (Smart Contract and NFT operations)
func (c *Core) processContractTransaction(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, currentOwner string) error {
	// Add token to database first
	deployerDID := txnBlock.GetDeployerDID()

	// Handle token generated type (new deployments)
	if newEvent.TxnType == block.TokenGeneratedType {
		if currentOwner != newEvent.PublisherDID {
			return fmt.Errorf("publisher DID mismatch for contract deployment: expected %s, got %s", currentOwner, newEvent.PublisherDID)
		}

		// Add block directly to token chain for new deployments
		if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
			return fmt.Errorf("failed to add contract block to token chain: %v", err)
		}

		c.log.Info("New contract deployment processed", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
		return nil
	}
	syncStatus := wallet.SyncCompleted
	if err := c.AddTokenToRespectiveTable(tokenId, deployerDID, txnBlock, newEvent, syncStatus); err != nil {
		return fmt.Errorf("failed to add contract token to table: %v", err)
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
		return fmt.Errorf("failed to get latest block for contract token %s - may need sync", tokenId)
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
	if previousOwner != newEvent.PublisherDID {
		return fmt.Errorf("contract publisher DID mismatch: expected %s, got %s",
			previousOwner, newEvent.PublisherDID)
	}

	// Add validated block to contract chain
	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add contract execution block to chain: %v", err)
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
