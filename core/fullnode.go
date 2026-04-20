package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/consensus"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// Enhanced subscription setup with error handling
func (c *Core) SubscribeTxnSetup() {
	// Initialize the transaction processor
	c.initDynamicTxnProcessor()

	topic := constants.Event_RubixTxns
	err := c.ps.SubscribeTopic(topic, c.TxnCallBack)
	if err != nil {
		// If already subscribed, this is expected when SetupQuorum is called
		// for multiple quorum DIDs on the same node. Not an error.
		if err.Error() == "topic already subscribed" {
			c.log.Debug("SubscribeTxnSetup: already subscribed to topic, skipping", "topic", topic)
			return
		}
		c.log.Error("Unable to subscribe to topic", "topic", topic, "error", err)
		return
	}
	c.log.Info("Successfully subscribed to topic: " + topic)
}

// Enhanced callback with dynamic scaling integration
func (c *Core) TxnCallBack(peerID string, topic string, data []byte) {
	var newEvent models.EventTransaction
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to parse published event", "error", err, "data", string(data))
		return
	}

	// Ignore failed transaction IDs
	// Valid condition for both full node and Quorum nodes
	if !newEvent.Status {
		return
	}

	var txInfo models.TransactionInfo
	if err := json.Unmarshal(newEvent.Transaction.Info, &txInfo); err != nil {
		c.log.Error(fmt.Sprintf("failed to unmarshal transaction info, err: %v", err))
		return
	}

	// If the current node is setup as quorum, we check the records in token_state_hashes
	// table to see if any previous transaction id from TransactionInfo is present in the
	// current node's table. If so, then its removed
	if len(c.qc) > 0 {
		// Use the first quorum DID registered on this node for unpledge callback
		var quorumDID string
		for did := range c.qc {
			quorumDID = did
			break
		}
		if err := c.CallBackQuorumUnpledge(newEvent.Transaction, quorumDID); err != nil {
			c.log.Error(fmt.Sprintf("failed to check token state hashes records, err: %v", err))
		}
	}

	// add publisher to peer did table
	publisherDetails := models.DID{
		DID:    txInfo.Initiator,
		PeerID: peerID,
	}
	err = c.AddPeerDetails(publisherDetails)
	if err != nil {
		c.log.Error("failed to add publisher info to DB", "err", err)
	}

	// Check for duplicate transactions — only mark as seen AFTER successful queue.
	// Using Load first to cheaply reject known duplicates without blocking.
	if _, exists := c.txnProcessor.processedTxns.Load(newEvent.TransactionID); exists {
		c.log.Info("Duplicate transaction ignored", "txnID", newEvent.TransactionID)
		return
	}

	// Update queue length metric for dynamic scaling
	currentQueueLen := int64(len(c.txnProcessor.txnQueue))
	atomic.StoreInt64(&c.txnProcessor.queueLength, currentQueueLen)

	// Queue transaction for processing with enhanced timeout handling
	select {
	case c.txnProcessor.txnQueue <- &newEvent:
		// Mark as seen only AFTER successful enqueue to prevent silent loss
		c.txnProcessor.processedTxns.Store(newEvent.TransactionID, time.Now())
		atomic.AddInt64(&c.txnProcessor.processedTxnCount, 1)
		c.log.Debug("Transaction queued successfully",
			"txnID", newEvent.TransactionID,
			"queueLength", currentQueueLen)

	case <-time.After(5 * time.Second):
		c.log.Error("Failed to queue transaction - queue full, will retry on next delivery",
			"txnID", newEvent.TransactionID,
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
func (c *Core) processTxnWithRetry(txnEvent *models.EventTransaction, workerID int) {
	if txnEvent == nil {
		c.log.Debug("processTxnWithRetry: txn event is nil")
		return
	}

	var lastErr error

	for attempt := 0; attempt < c.txnProcessor.maxRetries; attempt++ {
		if attempt > 0 {
			c.log.Info("Retrying transaction processing",
				"txnID", txnEvent.TransactionID,
				"attempt", attempt+1,
				"workerID", workerID)
			time.Sleep(c.txnProcessor.retryDelay * time.Duration(attempt))
		}

		err := c.processSingleTransaction(txnEvent)
		if err == nil {
			c.log.Info("Transaction processed successfully",
				"txnID", txnEvent.TransactionID,
				"workerID", workerID)
			return
		}

		lastErr = err
		c.log.Error("Transaction processing failed",
			"txnID", txnEvent.TransactionID,
			"attempt", attempt+1,
			"error", err,
			"workerID", workerID)
	}

	// All retries exhausted — remove from dedup map so future pubsub
	// re-deliveries can attempt processing again (conditions may change,
	// e.g. peer becomes reachable for chain sync).
	c.txnProcessor.processedTxns.Delete(txnEvent.TransactionID)

	c.handleFailedTransaction(txnEvent, lastErr)
}

// processSingleTransaction validates and stores a transaction to the DB.
func (c *Core) processSingleTransaction(newEvent *models.EventTransaction) error {
	txn := newEvent.Transaction
	if txn == nil {
		return fmt.Errorf("processSingleTransaction: transaction payload is nil")
	}
	if txn.ID == "" {
		return fmt.Errorf("processSingleTransaction: transaction id is empty")
	}
	if newEvent.TransactionID != "" && newEvent.TransactionID != txn.ID {
		return fmt.Errorf("processSingleTransaction: event transaction_id %q does not match transaction.id %q", newEvent.TransactionID, txn.ID)
	}

	//validate the transaction
	//First unMarshal the transaction info
	transactionInfo := &models.TransactionInfo{}
	err := json.Unmarshal(txn.Info, transactionInfo)
	if err != nil {
		c.log.Error("processSingleTransaction:failed to unmarshal transaction info", "error", err)
		return fmt.Errorf("processSingleTransaction: failed to unmarshal transaction info: %w", err)
	}
	initiatorDIDCrypto, err := c.InitialiseDID(transactionInfo.Initiator)
	if err != nil {
		c.log.Error("processSingleTransaction:failed to initialise initiator DID", "error", err)
		return fmt.Errorf("processSingleTransaction: failed to initialise initiator DID: %w", err)
	}
	quorumDCs := make(map[string]types.DIDCrypto, len(transactionInfo.Quorums))
	for _, quorum := range transactionInfo.Quorums {
		quorumDIDCrypto, err := c.InitialiseDID(quorum.Did)
		if err != nil {
			c.log.Error("processSingleTransaction:failed to initialise quorum DID", "error", err)
			return fmt.Errorf("processSingleTransaction: failed to initialise quorum DID: %w", err)
		}
		quorumDCs[quorum.Did] = quorumDIDCrypto
	}

	syncTxChains := func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error {
		return c.SyncTransactionChainsFromPeer(peerDID, tokenIDs, prevTxIDs, excludeTxIDs, c.fullNode)
	}
	isTransactionInfoValidated, err := consensus.ValidateTransaction(txn, c.fullNode, c.w, c.log, initiatorDIDCrypto, quorumDCs, c.testnet, c.mainnet, c.localnet, c.checkTokenStateHashPinned, syncTxChains)
	if err != nil {
		c.log.Error("processSingleTransaction:failed to validate transaction", "error", err)
		return fmt.Errorf("processSingleTransaction: failed to validate transaction: %w", err)
	}
	if !isTransactionInfoValidated {
		validationErr := fmt.Errorf("transaction validation failed")
		c.log.Error("processSingleTransaction:failed to validate transaction", "error", validationErr)

		if persistErr := c.w.StoreInvalidTransaction(txn, validationErr.Error()); persistErr != nil {
			c.log.Error("processSingleTransaction:failed to persist invalid transaction", "error", persistErr)
		}

		return fmt.Errorf("processSingleTransaction: failed to validate transaction: %w", validationErr)
	}

	//store the transaction
	c.log.Debug("processSingleTransaction: about to persist fullnode transaction", txn, "TransactionInfo", transactionInfo)
	if err := c.w.PersistFullNodeTransaction(c.w.Ctx, &wallet.FullNodePersistenceRequest{
		Transaction:     txn,
		TransactionInfo: transactionInfo,
	}); err != nil {
		c.log.Error("processSingleTransaction:failed to persist fullnode transaction", "error", err, "transaction_id", txn.ID)
		return fmt.Errorf("processSingleTransaction: failed to persist fullnode transaction: %w", err)
	}

	return nil
}

// Handle failed transactions after all retries
func (c *Core) handleFailedTransaction(txnEvent *models.EventTransaction, lastErr error) {
	c.log.Error("Transaction processing failed permanently",
		"txnID", txnEvent.TransactionID,
		"error", lastErr)

	failedTxn := &model.FailedTransaction{
		TxnID:      txnEvent.TransactionID,
		Error:      lastErr.Error(),
		FailedAt:   time.Now(),
		RetryCount: c.txnProcessor.maxRetries,
	}

	if err := c.w.StoreFailedTransaction(failedTxn); err != nil {
		c.log.Error("Failed to store failed transaction", "txnID", txnEvent.TransactionID, "error", err)
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

func (c *Core) checkTokenStateHashPinned(tokenID string, previousTransactionID string) error {
	if previousTransactionID == "" {
		return nil
	}

	tokenStateHash := tokenID + "." + previousTransactionID

	record, err := c.ipfsProviderStore.GetProviderByCID(tokenStateHash)
	if err != nil {
		return fmt.Errorf("failed to check pin status for %s: %w", tokenStateHash, err)
	}

	if record != nil {
		return fmt.Errorf("token %s is already pinned", tokenStateHash)
	}

	return nil
}
