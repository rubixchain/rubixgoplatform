package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/consensus"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// Enhanced subscription setup with error handling
func (c *Core) SubscribeTxnSetup() {
	// Initialize the transaction processor
	if c.fullNode {
		c.initDynamicTxnProcessor()
	}

	// Register the sync-token-chain route on the libp2p listener.
	c.l.AddRoute(setup.APISyncTransactionInfoFromFullnode, "POST", c.syncTransactionInfoFromFullnodeOverLibp2p)

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

	if c.fullNode {
		// Cheaply reject already-seen transactions without blocking.
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
			// Mark as seen only AFTER successful enqueue so a failed send
			// doesn't permanently poison the dedup map.
			c.txnProcessor.processedTxns.Store(newEvent.TransactionID, time.Now())
			atomic.AddInt64(&c.txnProcessor.processedTxnCount, 1)
			c.log.Debug("Transaction queued successfully",
				"txnID", newEvent.TransactionID,
				"queueLength", currentQueueLen)

		case <-time.After(10 * time.Second):
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

	// If the terminal failure is a validation failure, persist it once to the
	// invalid transactions table for audit. Doing this only here (instead of
	// inside processSingleTransaction) avoids writing the same row once per
	// retry attempt.
	if lastErr != nil &&
		txnEvent.Transaction != nil &&
		strings.Contains(lastErr.Error(), "failed to validate transaction") {
		if persistErr := c.w.StoreInvalidTransaction(txnEvent.Transaction, lastErr.Error()); persistErr != nil {
			c.log.Error("processTxnWithRetry: failed to persist invalid transaction",
				"txnID", txnEvent.TransactionID,
				"error", persistErr)
		}
	}
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
		return c.SyncTransactionChainsFromPeer(peerDID, tokenIDs, prevTxIDs, excludeTxIDs, false, c.fullNode)
	}
	// Fullnode trusts the quorum's earlier transfer-auth decision; the flag
	// is not in the EventTransaction.
	_, err = consensus.ValidateTransaction(txn, c.fullNode, c.w, c.log, initiatorDIDCrypto, quorumDCs, c.testnet, c.mainnet, c.localnet, c.checkTokenStateHashPinned, syncTxChains, false)
	if err != nil {
		c.log.Error("processSingleTransaction:failed to validate transaction", "error", err)
		// Storing the invalid transaction is deferred to processTxnWithRetry,
		// which records it once after all retries are exhausted instead of on
		// every attempt.
		return fmt.Errorf("processSingleTransaction: failed to validate transaction: %w", err)
	}

	//store the transaction
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

// GetTransactionInfoFromFullnode returns the transaction chain for each
// requested token ID. Tokens that fail to load are logged and skipped.
func (c *Core) GetTransactionInfoFromFullnode(tokenIDs []string) (map[string][]types.SyncedTxn, error) {
	result := make(map[string][]types.SyncedTxn)
	for _, tokenID := range tokenIDs {
		chain, err := c.w.GetFullNodeSyncedChain(tokenID)
		if err != nil {
			c.log.Warn("GetTransactionInfoFromFullnode: failed to fetch fullnode chain", "tokenID", tokenID, "err", err)
			continue
		}
		result[tokenID] = chain
	}

	return result, nil
}

// syncTransactionInfoFromFullnodeOverLibp2p handles the sync-token-chain
// request received over the libp2p listener.
func (c *Core) syncTransactionInfoFromFullnodeOverLibp2p(req *ensweb.Request) *ensweb.Result {
	var syncReq types.SyncTransactionInfoFromFullnodeRequest
	if err := c.l.ParseJSON(req, &syncReq); err != nil {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) == 0 {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: true, Message: "no token_ids provided"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) > 50 {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "max 50 token IDs per request"}, http.StatusOK)
	}
	data, err := c.GetTransactionInfoFromFullnode(syncReq.TokenIDs)
	if err != nil {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: err.Error()}, http.StatusOK)
	}
	return c.l.RenderJSON(req, &model.BasicResponse{Status: true, Message: "ok", Result: data}, http.StatusOK)
}
