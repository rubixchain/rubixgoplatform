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
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// Enhanced subscription setup with error handling
func (c *Core) SubscribeTxnSetup() {
	// Only fullnodes serve these endpoints
	if c.fullNode {
		c.initDynamicTxnProcessor()
		c.l.AddRoute(setup.APISyncTransactionInfoFromFullnode, "POST", c.syncTransactionInfoFromFullnode)
		c.registerRecoveryRoute()
	}

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
		Local:  peerID == c.peerID, // This was empty and was getting updated to false by default, so we set it based on the peerID comparison
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
	syncAuthoritative := func(tokenIDs []string) (map[string]string, error) {
		return c.syncTokensFromFullnode(tokenIDs)
	}
	getTxByID := func(txID string) (*models.TransactionInfo, error) {
		return c.getTransactionInfoByID(txID)
	}
	getParentBurnTx := func(parentID string) (string, bool, error) {
		return c.getParentBurnTxID(parentID)
	}
	fetchGenesisTx := func(peerDID, tokenID string) (*models.Transactions, error) {
		return c.FetchGenesisTransactionFromPeer(peerDID, tokenID)
	}
	// Fullnode trusts the quorum's earlier transfer-auth decision; the flag
	// is not in the EventTransaction.
	_, err = consensus.ValidateTransaction(txn, c.fullNode, c.w, c.log, initiatorDIDCrypto, quorumDCs, c.testnet, c.mainnet, c.localnet, c.checkTokenStateHashPinned, syncTxChains, syncAuthoritative, getTxByID, getParentBurnTx, fetchGenesisTx, false)
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

// Safety limits for the sync-txn-info-chain endpoint. PageSize is a server
// constant so total_pages stays stable across a single download run.
const (
	syncDefaultPageSize     = 100         // chain entries per page when caller omits it
	syncMaxPageSize         = 1000        // upper bound on caller-supplied page_size
	maxSyncTokensPerReq     = 50          // max token_ids a single request may carry
	syncMaxRequestBodyBytes = 64 * 1024   // request body size cap
	syncMaxOffsetRows       = 100_000_000 // refuse to OFFSET past this many rows
)

// syncTransactionInfoFromFullnode serves the libp2p endpoint an explorer (or
// any peer) calls to fetch chain entries for a list of tokens. Pagination
// is by absolute page_number — the caller can detect a missed page later
// and re-fetch just that page by its number. Full contract is on
// types.SyncTransactionInfoFromFullnodeRequest.
func (c *Core) syncTransactionInfoFromFullnode(req *ensweb.Request) *ensweb.Result {
	if httpReq := req.GetHTTPRequest(); httpReq != nil && httpReq.Body != nil {
		httpReq.Body = http.MaxBytesReader(req.GetHTTPWritter(), httpReq.Body, syncMaxRequestBodyBytes)
	}

	var syncReq types.SyncTransactionInfoFromFullnodeRequest
	if err := c.l.ParseJSON(req, &syncReq); err != nil {
		c.log.Debug("syncTransactionInfoFromFullnode: parse request body failed", "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) == 0 {
		return c.l.RenderJSON(req, &models.BasicResponse{Status: true, Message: "no token_ids provided"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) > maxSyncTokensPerReq {
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: fmt.Sprintf("max %d token IDs per request", maxSyncTokensPerReq)}, http.StatusOK)
	}

	// Dedup token_ids and drop any empty entries so they don't skew the count
	// or waste a slot.
	tokenIDs := make([]string, 0, len(syncReq.TokenIDs))
	seen := make(map[string]struct{}, len(syncReq.TokenIDs))
	for _, t := range syncReq.TokenIDs {
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		tokenIDs = append(tokenIDs, t)
	}
	if len(tokenIDs) == 0 {
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "token_ids contains no non-empty values"}, http.StatusOK)
	}

	// Clamp page size; default when zero. Page size must stay constant
	// across a single download so total_pages is stable.
	pageSize := syncReq.PageSize
	if pageSize <= 0 {
		pageSize = syncDefaultPageSize
	}
	if pageSize > syncMaxPageSize {
		pageSize = syncMaxPageSize
	}

	// Default to page 1 when the request omits page_number. We validate
	// upper bound after counting total_pages.
	pageNumber := syncReq.PageNumber
	if pageNumber <= 0 {
		pageNumber = 1
	}

	// Find tokens whose KnownPositions claim doesn't match the fullnode's
	// chain at that position. They're reported back in DivergentTokens and
	// get their full chain (the count/page queries see them as having no
	// known position).
	divergent, err := c.w.DetectDivergentSyncTokens(syncReq.KnownPositions)
	if err != nil {
		c.log.Warn("syncTransactionInfoFromFullnode: divergence check failed", "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode divergence check failed"}, http.StatusOK)
	}
	divergentSet := make(map[string]struct{}, len(divergent))
	for _, t := range divergent {
		divergentSet[t] = struct{}{}
	}
	// Build the thresholds map for the count/page queries: non-divergent
	// tokens contribute their claimed position; divergent tokens are left
	// out so the queries default them to -1 (= full chain).
	thresholds := make(map[string]int64, len(syncReq.KnownPositions))
	for tokenID, tip := range syncReq.KnownPositions {
		if _, isDivergent := divergentSet[tokenID]; isDivergent {
			continue
		}
		thresholds[tokenID] = tip.Position
	}

	totalItems, err := c.w.CountFullNodeSyncedChainEntries(tokenIDs, thresholds)
	if err != nil {
		c.log.Warn("syncTransactionInfoFromFullnode: count failed", "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode count failed"}, http.StatusOK)
	}
	totalPages := 0
	if totalItems > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}

	// Nothing to send: still return success with empty data so the caller
	// knows it's fully in sync.
	if totalItems == 0 {
		result := types.SyncTransactionInfoFromFullnodeResult{
			Data:            map[string][]types.SyncedTxn{},
			DivergentTokens: divergent,
			PageNumber:      pageNumber,
			TotalPages:      0,
			PageSize:        pageSize,
			TotalItems:      0,
		}
		return c.l.RenderJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
	}

	// Out-of-range page numbers are an obvious client bug — reject loudly
	// instead of returning an empty page.
	if pageNumber > totalPages {
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: fmt.Sprintf("page_number %d exceeds total_pages %d", pageNumber, totalPages)}, http.StatusOK)
	}

	offset := (pageNumber - 1) * pageSize
	if offset > syncMaxOffsetRows {
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: fmt.Sprintf("page offset would exceed safety cap (%d rows)", syncMaxOffsetRows)}, http.StatusOK)
	}

	keys, entries, err := c.w.GetFullNodeSyncedChainPageByOffset(tokenIDs, thresholds, offset, pageSize)
	if err != nil {
		c.log.Warn("syncTransactionInfoFromFullnode: page fetch failed",
			"page_number", pageNumber, "page_size", pageSize, "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: err.Error()}, http.StatusOK)
	}

	// Group entries by token_id for the response. Order within each token
	// stays correct because the query already sorts by (token_id, position).
	data := make(map[string][]types.SyncedTxn, len(tokenIDs))
	for i := range entries {
		data[keys[i]] = append(data[keys[i]], entries[i])
	}

	result := types.SyncTransactionInfoFromFullnodeResult{
		Data:            data,
		DivergentTokens: divergent,
		PageNumber:      pageNumber,
		TotalPages:      totalPages,
		PageSize:        pageSize,
		TotalItems:      totalItems,
	}
	return c.l.RenderJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
}
