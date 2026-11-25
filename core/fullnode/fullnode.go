package fullnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/consensus"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// QueueFullnodeTransaction admits a transaction and hands it to the worker pool.
// Admission is reserved before the send because pubsub delivers each message on
// its own goroutine, so two deliveries could otherwise both pass the check. Any
// path that fails to hand the event to a worker releases the reservation.
func (p *DynamicTxnProcessor) QueueFullnodeTransaction(newEvent *models.EventTransaction) {
	// Cheaply reject already-seen transactions without blocking.
	if !p.admit(newEvent.TransactionID) {
		p.host.Log().Info("Duplicate transaction ignored", "txnID", newEvent.TransactionID)
		return
	}

	// Update queue length metric for dynamic scaling
	currentQueueLen := int64(len(p.txnQueue))
	atomic.StoreInt64(&p.queueLength, currentQueueLen)

	// Mark as queued before the send: a worker removes the mark as soon as it
	// receives the event, so marking afterwards could strand the ID. Failed
	// sends below clear it.
	p.queued.add(newEvent.TransactionID)

	// Queue transaction for processing with enhanced timeout handling
	select {
	case p.txnQueue <- newEvent:
		atomic.AddInt64(&p.processedTxnCount, 1)
		p.host.Log().Debug("Transaction queued successfully",
			"txnID", newEvent.TransactionID,
			"queueLength", currentQueueLen)

	case <-time.After(p.enqueueTimeout):
		// Release admission so a re-delivery can try again instead of being
		// rejected as a duplicate.
		p.queued.remove(newEvent.TransactionID)
		p.releaseAdmission(newEvent.TransactionID)
		p.host.Log().Error("Failed to queue transaction - queue full, will retry on next delivery",
			"txnID", newEvent.TransactionID,
			"queueLength", len(p.txnQueue))

		if currentQueueLen > int64(p.queueThreshold) {
			p.host.Log().Warn("Queue threshold exceeded - scaling may be needed",
				"current", currentQueueLen,
				"threshold", p.queueThreshold)
		}

	case <-p.ctx.Done():
		p.queued.remove(newEvent.TransactionID)
		p.releaseAdmission(newEvent.TransactionID)
		p.host.Log().Info("Transaction processor shutting down")
	}
}

// Process transaction with retry mechanism
func (p *DynamicTxnProcessor) processTxnWithRetry(txnEvent *models.EventTransaction, workerID int) {
	if txnEvent == nil {
		p.host.Log().Debug("processTxnWithRetry: txn event is nil")
		return
	}

	// In flight for as long as this worker owns it. The defer also covers
	// panics, which dynamicWorker recovers from.
	entry := p.registerInflight(txnEvent)
	if entry != nil {
		defer p.unregisterInflight(entry.id)

		// Wait once, before the retry loop, not per attempt: a previous
		// transaction that never arrives would otherwise cost the wait each time.
		if err := p.awaitDependencies(entry); err != nil {
			// A previous transaction was found invalid: dead-letter this one
			// without validating it. Its own waiting transactions were already
			// failed by the same walk. Admission is not released: the verdict is
			// terminal and a re-delivery would only add a second dead-letter row.
			if errors.Is(err, errProducerFailed) {
				p.host.Log().Info("processTxnWithRetry: not validating, a producer of this transaction failed",
					"txnID", txnEvent.TransactionID, "workerID", workerID, "reason", err)
				p.storeInvalidTransaction(txnEvent, err)
				return
			}

			p.host.Log().Info("processTxnWithRetry: abandoning transaction before validation",
				"txnID", txnEvent.TransactionID, "reason", err)
			return
		}
	}

	var lastErr error
	for attempt := 0; attempt < p.maxRetries; attempt++ {
		if attempt > 0 {
			p.host.Log().Info("Retrying transaction processing",
				"txnID", txnEvent.TransactionID,
				"attempt", attempt+1,
				"workerID", workerID)
			time.Sleep(p.retryDelay * time.Duration(attempt))
		}

		err := p.processSingleTransaction(txnEvent)
		if err == nil {
			p.host.Log().Info("Transaction processed successfully",
				"txnID", txnEvent.TransactionID,
				"workerID", workerID)
			return
		}

		// A verdict reached while something it depends on is still pending
		// locally is downgraded to transient.
		err = p.deferVerdictWhileDependencyPending(entry, err)

		lastErr = err
		p.host.Log().Error("Transaction processing failed",
			"txnID", txnEvent.TransactionID,
			"attempt", attempt+1,
			"error", err,
			"workerID", workerID)

		// Retrying a verdict only delays it, and keeps waiting transactions
		// parked until their timers expire and failure propagation misses them.
		if errors.Is(err, errValidationFailed) {
			break
		}
	}

	if errors.Is(lastErr, errValidationFailed) {
		// Record the verdict once per transaction (not per attempt) and fail
		// the waiting transactions. Admission is not released: a re-delivery
		// would only reach the same verdict.
		p.storeInvalidTransaction(txnEvent, lastErr)
		p.failDownstream(txnEvent.TransactionID, lastErr)
		return
	}

	// Transient or unclassified: release admission so a re-delivery can try
	// again (e.g. once a peer is reachable). Nothing is dead-lettered and
	// waiting transactions are left to their own timers and retries.
	p.releaseAdmission(txnEvent.TransactionID)
}

// deferVerdictWhileDependencyPending re-tags a verdict as transient when a
// declared previous transaction is still pending, or a recent sync of one of its
// tokens was trimmed by GuardAgainstInflight. Either can make phase 3 report a
// local-ordering mismatch as a verdict. It only ever defers a verdict; if every
// attempt is deferred, admission is released and nothing is dead-lettered.
func (p *DynamicTxnProcessor) deferVerdictWhileDependencyPending(entry *inflightTxn, err error) error {
	if entry == nil || err == nil || !errors.Is(err, errValidationFailed) {
		return err
	}

	tokens := entryTokenIDs(entry)

	reason, detail := "", ""
	for _, dep := range entry.deps {
		if p.isPending(dep) {
			reason, detail = "a declared producer is still pending", dep
			break
		}
	}
	if reason == "" {
		if tokenID, trimmed := p.truncated.recentlyTruncated(tokens, truncationTTL); trimmed {
			reason, detail = "a chain sync for one of its tokens was trimmed", tokenID
		}
	}
	if reason == "" {
		return err
	}

	atomic.AddInt64(&p.verdictsDeferred, 1)
	p.host.Log().Info("processTxnWithRetry: deferring a verdict, "+reason,
		"txnID", entry.id, "dependency", detail, "reason", err)

	// The memoised sync may be the trimmed one; drop it so the retry re-syncs.
	p.invalidateSyncedTokens(tokens)

	return classify(errDependencyTimeout, stripClass(err))
}

// entryTokenIDs returns the tokens the entry's transaction touches, or nil when
// its payload cannot be read.
func entryTokenIDs(entry *inflightTxn) []string {
	if entry == nil || entry.event == nil || entry.event.Transaction == nil || len(entry.event.Transaction.Info) == 0 {
		return nil
	}
	var info models.TransactionInfo
	if err := json.Unmarshal(entry.event.Transaction.Info, &info); err != nil {
		return nil
	}
	return transactionTokenIDs(&info)
}

// storeInvalidTransaction records a terminal verdict in the audit table, using
// the error's message unchanged (classification is attached beside it).
func (p *DynamicTxnProcessor) storeInvalidTransaction(txnEvent *models.EventTransaction, cause error) {
	if txnEvent == nil || txnEvent.Transaction == nil || cause == nil {
		return
	}
	if err := p.host.Wallet().StoreInvalidTransaction(txnEvent.Transaction, cause.Error()); err != nil {
		p.host.Log().Error("processTxnWithRetry: failed to persist invalid transaction",
			"txnID", txnEvent.TransactionID,
			"error", err)
	}
}

// processSingleTransaction validates and stores a transaction to the DB.
func (p *DynamicTxnProcessor) processSingleTransaction(newEvent *models.EventTransaction) error {
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
		p.host.Log().Error("processSingleTransaction:failed to unmarshal transaction info", "error", err)
		return fmt.Errorf("processSingleTransaction: failed to unmarshal transaction info: %w", err)
	}
	initiatorDIDCrypto, err := p.host.InitialiseDID(transactionInfo.Initiator)
	if err != nil {
		p.host.Log().Error("processSingleTransaction:failed to initialise initiator DID", "error", err)
		return fmt.Errorf("processSingleTransaction: failed to initialise initiator DID: %w", err)
	}
	quorumDCs := make(map[string]types.DIDCrypto, len(transactionInfo.Quorums))
	for _, quorum := range transactionInfo.Quorums {
		quorumDIDCrypto, err := p.host.InitialiseDID(quorum.Did)
		if err != nil {
			p.host.Log().Error("processSingleTransaction:failed to initialise quorum DID", "error", err)
			return fmt.Errorf("processSingleTransaction: failed to initialise quorum DID: %w", err)
		}
		quorumDCs[quorum.Did] = quorumDIDCrypto
	}

	// Chain syncs go through the per-bundle sync memo. Pending transactions are
	// kept out by the guard in the apply path, not by excludeTxIDs, which would
	// make the peer return a chain with a hole in it.
	syncTxChains := func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error {
		return p.syncChainsOnce(txn.ID, peerDID, tokenIDs, prevTxIDs, excludeTxIDs)
	}
	syncAuthoritative := func(tokenIDs []string) (map[string]string, error) {
		return p.host.SyncTokensFromFullnode(tokenIDs)
	}
	getTxByID := func(txID string) (*models.TransactionInfo, error) {
		return p.host.GetTransactionInfoByID(txID)
	}
	getParentBurnTx := func(parentID string) (string, bool, error) {
		return p.host.GetParentBurnTxID(parentID)
	}
	fetchGenesisTx := func(peerDID, tokenID string) (*models.Transactions, error) {
		// A peer failure is transient, not a verdict on the transaction.
		txn, err := p.host.FetchGenesisTransactionFromPeer(peerDID, tokenID)
		return txn, classify(errDependencyTimeout, err)
	}
	// Cache terminal ancestor chains through the host; its apply path also
	// respects transactions still pending in this processor.
	syncBurntChain := func(peerDID, tokenID string) error {
		return p.host.SyncBurntTokenChainFromPeer(peerDID, tokenID)
	}
	// Fullnode trusts the quorum's earlier transfer-auth decision; the flag
	// is not in the EventTransaction.
	testnet, mainnet, localnet := p.host.NetworkFlags()
	_, err = consensus.ValidateTransaction(txn, p.host.IsFullNode(), p.host.Wallet(), p.host.Log(), initiatorDIDCrypto, quorumDCs, testnet, mainnet, localnet, p.host.CheckTokenStateHashPinned, syncTxChains, syncAuthoritative, getTxByID, getParentBurnTx, fetchGenesisTx, syncBurntChain, p.host.VerifyGenesisSignature, false)
	if err != nil {
		p.host.Log().Error("processSingleTransaction:failed to validate transaction", "error", err)
		// processTxnWithRetry stores the invalid transaction, once. classify
		// attaches the class beside the error so its text, including "failed to
		// validate transaction", stays byte-identical.
		return classify(
			classifyValidationFailure(err),
			fmt.Errorf("processSingleTransaction: failed to validate transaction: %w", err),
		)
	}

	//store the transaction
	if err := p.host.Wallet().PersistFullNodeTransaction(p.host.Wallet().Ctx, &wallet.FullNodePersistenceRequest{
		Transaction:     txn,
		TransactionInfo: transactionInfo,
	}); err != nil {
		p.host.Log().Error("processSingleTransaction:failed to persist fullnode transaction", "error", err, "transaction_id", txn.ID)
		return fmt.Errorf("processSingleTransaction: failed to persist fullnode transaction: %w", err)
	}

	// The tips just advanced, so invalidate the memo BEFORE waking waiters: a
	// woken transaction validates immediately and must not skip a needed sync.
	tokenIDs := transactionTokenIDs(transactionInfo)
	p.invalidateSyncedTokens(tokenIDs)
	p.truncated.forget(tokenIDs)

	// Wake waiting transactions only after the persist commits: one woken
	// earlier would not find the row and has only one wake-up. On a failed
	// persist nobody is woken and waiters fall back to their timers.
	p.releaseWaiters(txn.ID)

	return nil
}

// Graceful shutdown
func (p *DynamicTxnProcessor) ShutdownTxnProcessor() {
	if p == nil {
		return
	}
	{
		p.host.Log().Info("Shutting down transaction processor")
		p.cancel()

		// Close the queue channel to signal workers to finish current work
		close(p.txnQueue)

		// Wait for all workers to complete with timeout
		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			p.host.Log().Info("All transaction workers shut down gracefully")
		case <-time.After(30 * time.Second):
			p.host.Log().Warn("Transaction workers shutdown timeout - forcing termination")
		}
	}
}

// RegisterRoutes publishes the endpoints only a fullnode serves. Called from
// Core.SubscribeTxnSetup for the fullnode role.
func (p *DynamicTxnProcessor) RegisterRoutes() {
	p.host.Listener().AddRoute(setup.APISyncTransactionInfoFromFullnode, "POST", p.syncTransactionInfoFromFullnode)
	p.registerRecoveryRoute()
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
func (p *DynamicTxnProcessor) syncTransactionInfoFromFullnode(req *ensweb.Request) *ensweb.Result {
	if httpReq := req.GetHTTPRequest(); httpReq != nil && httpReq.Body != nil {
		httpReq.Body = http.MaxBytesReader(req.GetHTTPWritter(), httpReq.Body, syncMaxRequestBodyBytes)
	}

	var syncReq types.SyncTransactionInfoFromFullnodeRequest
	if err := p.host.Listener().ParseJSON(req, &syncReq); err != nil {
		p.host.Log().Debug("syncTransactionInfoFromFullnode: parse request body failed", "err", err)
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) == 0 {
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: true, Message: "no token_ids provided"}, http.StatusOK)
	}
	if len(syncReq.TokenIDs) > maxSyncTokensPerReq {
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: fmt.Sprintf("max %d token IDs per request", maxSyncTokensPerReq)}, http.StatusOK)
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
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: "token_ids contains no non-empty values"}, http.StatusOK)
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
	divergent, err := p.host.Wallet().DetectDivergentSyncTokens(syncReq.KnownPositions)
	if err != nil {
		p.host.Log().Warn("syncTransactionInfoFromFullnode: divergence check failed", "err", err)
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode divergence check failed"}, http.StatusOK)
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

	totalItems, err := p.host.Wallet().CountFullNodeSyncedChainEntries(tokenIDs, thresholds)
	if err != nil {
		p.host.Log().Warn("syncTransactionInfoFromFullnode: count failed", "err", err)
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode count failed"}, http.StatusOK)
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
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
	}

	// Out-of-range page numbers are an obvious client bug — reject loudly
	// instead of returning an empty page.
	if pageNumber > totalPages {
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: fmt.Sprintf("page_number %d exceeds total_pages %d", pageNumber, totalPages)}, http.StatusOK)
	}

	offset := (pageNumber - 1) * pageSize
	if offset > syncMaxOffsetRows {
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: fmt.Sprintf("page offset would exceed safety cap (%d rows)", syncMaxOffsetRows)}, http.StatusOK)
	}

	keys, entries, err := p.host.Wallet().GetFullNodeSyncedChainPageByOffset(tokenIDs, thresholds, offset, pageSize)
	if err != nil {
		p.host.Log().Warn("syncTransactionInfoFromFullnode: page fetch failed",
			"page_number", pageNumber, "page_size", pageSize, "err", err)
		return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: false, Message: err.Error()}, http.StatusOK)
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
	return p.host.Listener().RenderJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
}
