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
//
// Split out of TxnCallBack so the admission/enqueue interaction can be tested
// without a database — everything above this point in TxnCallBack needs a wallet.
//
// Admission is reserved up front rather than recorded after a successful send.
// The previous order (check, enqueue, then mark) was a check-then-act race:
// pubsub gives every message its own goroutine (types/pubsub.go:164), so two
// deliveries of the same transaction could both pass the check and both enqueue.
// Reserving first closes that window, and every path that fails to hand the event
// to a worker releases the reservation, which preserves the original invariant
// that a failed send must not poison the dedup map.
func (p *DynamicTxnProcessor) QueueFullnodeTransaction(newEvent *models.EventTransaction) {
	// Cheaply reject already-seen transactions without blocking.
	if !p.admit(newEvent.TransactionID) {
		p.host.Log().Info("Duplicate transaction ignored", "txnID", newEvent.TransactionID)
		return
	}

	// Update queue length metric for dynamic scaling
	currentQueueLen := int64(len(p.txnQueue))
	atomic.StoreInt64(&p.queueLength, currentQueueLen)

	// Queue transaction for processing with enhanced timeout handling
	select {
	case p.txnQueue <- newEvent:
		atomic.AddInt64(&p.processedTxnCount, 1)
		p.host.Log().Debug("Transaction queued successfully",
			"txnID", newEvent.TransactionID,
			"queueLength", currentQueueLen)

	case <-time.After(p.enqueueTimeout):
		// No worker ever saw this event, so give up the reservation: a later
		// re-delivery has to be free to try again instead of being rejected as a
		// duplicate until dedupMapCleaner sweeps the entry.
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

	// Mark the transaction as in flight for exactly as long as this worker owns
	// it. The defer is what guarantees that: dynamicWorker recovers from panics
	// (fullnode_txn_processor.go, dynamicWorker), so an early return or a panic
	// must not leave a stale entry behind.
	if entry := p.registerInflight(txnEvent); entry != nil {
		defer p.unregisterInflight(entry.id)

		// Wait here, once, rather than inside the retry loop below: a producer
		// that never arrives would otherwise cost the wait on every attempt.
		// An expired wait returns nil and the transaction proceeds normally.
		if err := p.awaitDependencies(entry); err != nil {
			// A producer found invalid is a verdict on this transaction too,
			// reached without validating it: it spends an output of something
			// that never legitimately existed. Dead-letter it and stop. Its own
			// consumers were failed by the same walk that failed this one, so
			// there is nothing further to propagate from here.
			//
			// Admission is deliberately not released. The verdict is terminal,
			// and letting a re-delivery back in would only produce a second
			// dead-letter row for the same conclusion.
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

		lastErr = err
		p.host.Log().Error("Transaction processing failed",
			"txnID", txnEvent.TransactionID,
			"attempt", attempt+1,
			"error", err,
			"workerID", workerID)

		// A verdict does not change on re-reading the same data, so the
		// remaining attempts would only postpone it — and everything parked on
		// this transaction stays parked while they run, which is what would
		// leave the propagation below with nobody left to reach.
		if errors.Is(err, errValidationFailed) {
			break
		}
	}

	if errors.Is(lastErr, errValidationFailed) {
		// Terminal, and this node's own conclusion. Record it once — here
		// rather than inside processSingleTransaction, so a row is written per
		// transaction and not per attempt — and fail everything that was
		// waiting to build on it.
		//
		// Admission is not released: the verdict will not change, so a
		// re-delivery would only reach it again.
		p.storeInvalidTransaction(txnEvent, lastErr)
		p.failDownstream(txnEvent.TransactionID, lastErr)
		return
	}

	// Transient, or no verdict at all. Release the admission so future pubsub
	// re-deliveries can attempt processing again — conditions may change, for
	// instance a peer becoming reachable for chain sync. Nothing is
	// dead-lettered and nothing downstream is touched: this says nothing about
	// the transaction, and the consumers waiting on it keep their own retries.
	p.releaseAdmission(txnEvent.TransactionID)
}

// storeInvalidTransaction records a terminal verdict in the audit table.
//
// The stored reason is the error's own message, which is why classification is
// attached beside the message rather than wrapped into it: what lands in this
// table reads exactly as it did before typed errors existed.
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

	// The sync-once gate. excludeTxIDs is passed straight through: the guard that
	// keeps an in-flight sibling's chain entry out lives one layer down, in the
	// apply loop, and duplicating it here as a peer-side exclusion would make the
	// peer return a chain with a hole in it rather than a shorter one.
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
		// Tagged transient for the same reason the chain sync is: this reaches
		// out to a peer, and a peer that cannot be reached is not a verdict on
		// the transaction.
		txn, err := p.host.FetchGenesisTransactionFromPeer(peerDID, tokenID)
		return txn, classify(errDependencyTimeout, err)
	}
	// Fullnode trusts the quorum's earlier transfer-auth decision; the flag
	// is not in the EventTransaction.
	testnet, mainnet, localnet := p.host.NetworkFlags()
	_, err = consensus.ValidateTransaction(txn, p.host.IsFullNode(), p.host.Wallet(), p.host.Log(), initiatorDIDCrypto, quorumDCs, testnet, mainnet, localnet, p.host.CheckTokenStateHashPinned, syncTxChains, syncAuthoritative, getTxByID, getParentBurnTx, fetchGenesisTx, false)
	if err != nil {
		p.host.Log().Error("processSingleTransaction:failed to validate transaction", "error", err)
		// Storing the invalid transaction is deferred to processTxnWithRetry,
		// which records it once instead of on every attempt.
		//
		// The message is unchanged, deliberately. classify attaches the verdict
		// as a second branch of the error tree rather than wrapping it into the
		// text, so "failed to validate transaction" still appears here byte for
		// byte and anything matching on it keeps working.
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

	// This node has just advanced the tip of every token the transaction
	// touched, so anything the memo remembers about them now describes a chain
	// one entry short. Forget it before waking anybody: a released waiter starts
	// validating immediately, and it must not be handed a reason to skip a sync
	// it now genuinely needs.
	p.invalidateSyncedTokens(transactionTokenIDs(transactionInfo))

	// This transaction is now a producer that has resolved, so wake anything
	// held behind it. The call sits here, after the persist returns, and not
	// anywhere earlier: PersistFullNodeTransaction writes the chain entry inside
	// its own database transaction, and a waiter woken before that commits would
	// re-probe, still not find the row, and have spent its one wake-up.
	//
	// Nothing is released when the persist fails. The waiters then fall back to
	// their timers and behave as they did before the cascade existed, which is
	// correct: there is no row for them to have been waiting for.
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
