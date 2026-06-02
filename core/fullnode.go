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
	// The transaction processor and the sync-txn-info-chain endpoint only make
	// sense on a fullnode — non-fullnodes don't have the fullnode_* tables
	// the endpoint reads from. Gating both keeps the libp2p surface small.
	if c.fullNode {
		c.initDynamicTxnProcessor()
		c.l.AddRoute(setup.APISyncTransactionInfoFromFullnode, "POST", c.syncTransactionInfoFromFullnode)
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

// Pagination budget and request safety caps for the sync-txn-info-chain endpoint.
const (
	syncResponseSizeBudget  = 2 * 1024 * 1024 // ~2 MB soft target response body size
	syncResponseHardCap     = 8 * 1024 * 1024 // 8 MB absolute response cap; even K=0 honors this
	syncPerTokenSafetyCap   = 100             // hard cap on entries fetched per token per call
	maxSyncTokensPerReq     = 50
	syncMaxRequestBodyBytes = 16 * 1024 // 16 KB cap on the request body; legitimate payloads are <5 KB
	syncMaxOffset           = 10_000_000

	// Fixed overhead in bytes added when estimating a SyncedTxn's marshalled
	// size. Covers id, role, previous_transaction_id, JSON keys, separators
	// — bounded by tokenchain row shape and well above worst case.
	syncedTxnFixedOverheadBytes = 256
)

// GetTransactionInfoFromFullnodePage returns a page of chain entries for the
// requested tokens. All tokens advance by the same K entries from `offset` to
// `offset+K`. K is chosen so the marshalled total stays under
// syncResponseSizeBudget; at least 1 entry is always returned per non-empty
// token so progress is guaranteed even when a single entry exceeds the budget.
// Per-token fetch errors are logged and that token is omitted from the result.
func (c *Core) GetTransactionInfoFromFullnodePage(tokenIDs []string, offset int) (
	data map[string][]types.SyncedTxn,
	advancedBy int,
	hasMore bool,
	err error,
) {
	type tokenState struct {
		entries        []types.SyncedTxn
		sizes          []int // marshalled size per entry, aligned with entries
		hasMorePastCap bool
	}

	states := make(map[string]*tokenState, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		entries, hasMoreCap, ferr := c.w.GetFullNodeSyncedChainPage(tokenID, offset, syncPerTokenSafetyCap)
		if ferr != nil {
			c.log.Warn("GetTransactionInfoFromFullnodePage: failed to fetch fullnode chain page",
				"tokenID", tokenID, "offset", offset, "err", ferr)
			continue
		}
		sizes := make([]int, len(entries))
		for i := range entries {
			// Info is already json.RawMessage from the DB, so its byte length
			// is exact. The fixed overhead bounds id/role/prev/JSON syntax.
			// Avoiding a per-entry json.Marshal here removes a full marshal
			// pass — RenderJSON does the only real marshal.
			sizes[i] = len(entries[i].Info) + syncedTxnFixedOverheadBytes
		}
		states[tokenID] = &tokenState{entries: entries, sizes: sizes, hasMorePastCap: hasMoreCap}
	}

	// Choose K: largest K within safety cap such that summed step sizes stay
	// under the soft budget AND the hard cap. The hard cap is enforced at
	// every K — including K=0 — so a pathological payload can never produce
	// a multi-hundred-MB response. The soft budget is bypassed only at K=0
	// to guarantee forward progress on long chains.
	K := 0
	total := 0
	exceededHardCapAtZero := false
	for K < syncPerTokenSafetyCap {
		stepSize := 0
		anyAdvance := false
		for _, st := range states {
			if K < len(st.entries) {
				stepSize += st.sizes[K]
				anyAdvance = true
			}
		}
		if !anyAdvance {
			break // all tokens drained within safety cap
		}
		if total+stepSize > syncResponseHardCap {
			if K == 0 {
				exceededHardCapAtZero = true
			}
			break
		}
		if K > 0 && total+stepSize > syncResponseSizeBudget {
			break
		}
		total += stepSize
		K++
	}

	if exceededHardCapAtZero {
		return nil, 0, false, fmt.Errorf(
			"sync response would exceed %d byte hard cap at K=0; request fewer token_ids per call",
			syncResponseHardCap,
		)
	}

	data = make(map[string][]types.SyncedTxn, len(states))
	for tokenID, st := range states {
		n := K
		if n > len(st.entries) {
			n = len(st.entries)
		}
		data[tokenID] = st.entries[:n]
		if n < len(st.entries) || st.hasMorePastCap {
			hasMore = true
		}
	}
	advancedBy = K
	return data, advancedBy, hasMore, nil
}

// syncTransactionInfoFromFullnode handles the sync-txn-info-chain request
// over the libp2p listener. Paginated by size: caller passes offset and
// re-requests with offset += advanced_by until has_more is false.
func (c *Core) syncTransactionInfoFromFullnode(req *ensweb.Request) *ensweb.Result {
	// Cap request body size for this endpoint only. Legitimate payloads are
	// well under 5 KB (50 token IDs + offset); anything larger is malformed
	// or malicious.
	if httpReq := req.GetHTTPRequest(); httpReq != nil && httpReq.Body != nil {
		httpReq.Body = http.MaxBytesReader(req.GetHTTPWritter(), httpReq.Body, syncMaxRequestBodyBytes)
	}

	var syncReq types.SyncTransactionInfoFromFullnodeRequest
	if err := c.l.ParseJSON(req, &syncReq); err != nil {
		c.log.Debug("syncTransactionInfoFromFullnode: parse request body failed", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) == 0 {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: true, Message: "no token_ids provided"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) > maxSyncTokensPerReq {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: fmt.Sprintf("max %d token IDs per request", maxSyncTokensPerReq)}, http.StatusOK)
	}

	// Dedup and drop empty IDs. A duplicate or empty entry would otherwise
	// run a redundant (or zero-result) DB query and waste a token slot.
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
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "token_ids contains no non-empty values"}, http.StatusOK)
	}

	offset := syncReq.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > syncMaxOffset {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: fmt.Sprintf("offset exceeds max %d", syncMaxOffset)}, http.StatusOK)
	}

	data, advancedBy, hasMore, err := c.GetTransactionInfoFromFullnodePage(tokenIDs, offset)
	if err != nil {
		c.log.Warn("syncTransactionInfoFromFullnode: page fetch failed",
			"offset", offset, "tokenCount", len(tokenIDs), "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: err.Error()}, http.StatusOK)
	}
	result := types.SyncTransactionInfoFromFullnodeResult{
		Data:       data,
		HasMore:    hasMore,
		AdvancedBy: advancedBy,
	}
	return c.l.RenderJSON(req, &model.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
}
