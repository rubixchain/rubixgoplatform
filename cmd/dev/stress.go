package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// stressResult captures the goroutine ID and error from a concurrent transfer attempt.
type stressResult struct {
	id  int
	err error
}

// buildTransferReq constructs a fully-signed PostConsensusPersistenceRequest by reading
// the current token state from the DB. Used by concurrency scenarios where the token state
// is not pre-fetched. Returns (nil, err) on any failure — callers handle the error.
func buildTransferReq(w *wallet.Wallet, cfg *types.RubixConfig, fromDID, toDID, tokenID string, epochOffset int64) (*wallet.PostConsensusPersistenceRequest, error) {
	tok, err := w.ReadToken(tokenID)
	if err != nil {
		return nil, fmt.Errorf("buildTransferReq: ReadToken(%s): %w", tokenID, err)
	}
	req, _, _, err := buildTransferReqFromTok(cfg, fromDID, toDID, tok, epochOffset)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// buildTransferReqFromTok constructs a PostConsensusPersistenceRequest from a pre-fetched
// token record. Returns (req, txInfo, sigHex, err) so fuzz cases can mutate fields before
// calling PersistPostConsensus.
func buildTransferReqFromTok(cfg *types.RubixConfig, fromDID, toDID string, tok *models.Token, epochOffset int64) (*wallet.PostConsensusPersistenceRequest, *models.TransactionInfo, string, error) {
	tokenID := tok.TokenID

	txInfo := &models.TransactionInfo{
		Initiator: fromDID,
		Owner:     toDID,
		Epoch:     int(time.Now().UnixNano() + epochOffset),
		Network:   "local",
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{
					TokenID:               tokenID,
					PreviousTransactionID: tok.TransactionID,
					TokenValue:            tok.TokenValue,
				},
			},
		},
	}

	dc := did.InitDIDLiteWithPassword(fromDID, cfg.DidDir, "pwd-1")
	sigHex, err := util.SignTransaction(dc, txInfo)
	if err != nil {
		return nil, nil, "", fmt.Errorf("buildTransferReqFromTok: SignTransaction: %w", err)
	}
	if err := util.VerifySignature(dc, txInfo, sigHex); err != nil {
		return nil, nil, "", fmt.Errorf("buildTransferReqFromTok: VerifySignature: %w", err)
	}

	infoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return nil, nil, "", fmt.Errorf("buildTransferReqFromTok: SerializeTransactionInfo: %w", err)
	}
	sigStruct := &models.Signature{InitiatorSignature: sigHex}
	sigBytes, err := json.Marshal(sigStruct)
	if err != nil {
		return nil, nil, "", fmt.Errorf("buildTransferReqFromTok: marshal signature: %w", err)
	}
	txID, err := util.GetTransactionID(txInfo)
	if err != nil {
		return nil, nil, "", fmt.Errorf("buildTransferReqFromTok: ComputeTransactionID: %w", err)
	}

	tx := &models.Transactions{
		ID:        txID,
		Info:      infoBytes,
		Signature: json.RawMessage(sigBytes),
	}

	newPos := tok.LatestPosition + 1
	prevTxID := tok.TransactionID
	transferRole := int16(models.GetTokenRoleID(constants.TokenRole_Transfer))

	req := &wallet.PostConsensusPersistenceRequest{
		Transaction:     tx,
		TransactionInfo: txInfo,
		Signature:       sigStruct,
		DID:             fromDID,
		ExecutionRole:   wallet.ExecutionRoleInitiator,
		AffectedTokens:  []string{tokenID},
		TokenChainRows: []models.TokenChain{
			{
				TokenID:               tokenID,
				TransactionID:         txID,
				PreviousTransactionID: &prevTxID,
				Role:                  transferRole,
				Position:              newPos,
			},
		},
		TokenStates: []models.Token{
			{
				TokenID:        tokenID,
				DID:            toDID,
				TransactionID:  txID,
				TokenValue:     tok.TokenValue,
				TokenStatus:    int16(constants.TokenStatus_Free),
				TokenType:      tok.TokenType,
				LatestPosition: newPos,
				LatestRole:     transferRole,
			},
		},
	}
	return req, txInfo, sigHex, nil
}

// ptrStr returns a pointer to the given string. Helper for fuzz cases.
func ptrStr(s string) *string {
	return &s
}

// RunStressTests runs concurrency and fuzz scenarios against the PostgreSQL persistence layer.
// It must be called after the main transfer simulation is complete.
// dids must have at least 3 elements; didTokens maps DID -> tokenIDs for dids[0] and dids[1].
func RunStressTests(w *wallet.Wallet, cfg *types.RubixConfig, dids []string, didTokens map[string][]string) {
	fmt.Println("\n=== STRESS TESTS ===")

	// -----------------------------------------------------------------------
	// Scenario 1: Double-spend (5 goroutines, 1 token)
	// -----------------------------------------------------------------------
	// NOTE: after the main transfer simulation, dids[0] no longer owns didTokens[dids[0]][0]
	// (it was transferred to dids[2]). We need to pick a token currently owned by dids[0].
	// However, per the PLAN, the stress tests use didTokens[dids[0]][0] as if it belongs to dids[0].
	// The 3-DID simulation transferred D1 tokens -> D3, so we read the actual owner from DB.
	// For S1, we use whatever DID currently owns tok0 as the sender.
	tok0 := didTokens[dids[0]][0]
	tok0State, err := w.ReadToken(tok0)
	if err != nil {
		log.Fatalf("[STRESS S1] cannot read token %s: %v", tok0, err)
	}
	s1FromDID := tok0State.DID
	// Pick a toDID that is different from s1FromDID
	s1ToDID := dids[0]
	if s1ToDID == s1FromDID {
		s1ToDID = dids[1]
	}
	if s1ToDID == s1FromDID {
		s1ToDID = dids[2]
	}

	fmt.Printf("[STRESS S1] double-spend: 5 goroutines on token=%s (owner=%s -> %s)\n", tok0, s1FromDID, s1ToDID)

	s1Results := make(chan stressResult, 5)
	var wg sync.WaitGroup

	for i := range 5 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					s1Results <- stressResult{id, fmt.Errorf("panic: %v", r)}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req, err := buildTransferReq(w, cfg, s1FromDID, s1ToDID, tok0, int64(id))
			if err != nil {
				s1Results <- stressResult{id, err}
				return
			}
			err = w.PersistPostConsensus(ctx, req)
			fmt.Printf("[STRESS S1 g%d] token=%s err=%v\n", id, tok0, err)
			s1Results <- stressResult{id, err}
		}(i)
	}
	wg.Wait()
	close(s1Results)

	var s1Succeeded, s1Failed int
	for r := range s1Results {
		if r.err == nil {
			s1Succeeded++
		} else {
			s1Failed++
		}
	}
	if s1Succeeded != 1 {
		tok, _ := w.ReadToken(tok0)
		chain, _ := w.GetTokenChainByTokenID(tok0, false)
		fmt.Printf("[STRESS S1 DEBUG] token state: %+v, chain len=%d\n", tok, len(chain))
		log.Fatalf("STRESS FAIL S1: expected 1 success, got %d successes, %d failures", s1Succeeded, s1Failed)
	}
	fmt.Printf("[STRESS S1] double-spend: %d success, %d failed — PASS\n", s1Succeeded, s1Failed)

	// Post-validate S1
	{
		tok, err := w.ReadToken(tok0)
		if err != nil {
			log.Fatalf("[STRESS S1] post-validate: ReadToken(%s): %v", tok0, err)
		}
		if tok.DID != s1ToDID {
			log.Fatalf("[STRESS S1] post-validate: token %s owner=%s, expected %s", tok0, tok.DID, s1ToDID)
		}
		chain, err := w.GetTokenChainByTokenID(tok0, false)
		if err != nil {
			log.Fatalf("[STRESS S1] post-validate: GetTokenChainByTokenID(%s): %v", tok0, err)
		}
		// The chain length for tok0: it was previously at some position P (after main sim),
		// now it should have exactly 1 more row.
		// We validate positions are contiguous starting at 0.
		for i, row := range chain {
			if row.Position != int64(i) {
				log.Fatalf("[STRESS S1] post-validate: token %s chain[%d] position=%d, expected %d", tok0, i, row.Position, i)
			}
		}
		// Verify no duplicate positions
		posSet := make(map[int64]bool)
		for _, row := range chain {
			if posSet[row.Position] {
				log.Fatalf("[STRESS S1] post-validate: duplicate position %d in chain for token %s", row.Position, tok0)
			}
			posSet[row.Position] = true
		}
		fmt.Printf("[STRESS S1] post-validate: token %s owner=%s chainLen=%d positions-contiguous — OK\n", tok0, tok.DID, len(chain))
	}

	// -----------------------------------------------------------------------
	// Scenario 2: Parallel independent transfers
	// -----------------------------------------------------------------------
	// After main sim, dids[1] tokens were transferred to dids[0], and then dids[0]
	// transferred its original tokens to dids[2]. We need to find tokens currently owned
	// by some DID. We read from DB to get the actual current owner.
	// Per the plan, s2 uses didTokens[dids[1]] (originally D2's tokens).
	// After main sim they were D2->D1->D3. So actual owner is dids[2].
	// We re-read to determine current owners and set up the transfer correctly.

	s2TokenPool := didTokens[dids[1]]
	if len(s2TokenPool) > 5 {
		s2TokenPool = s2TokenPool[:5]
	}

	// Determine a valid (fromDID, toDID) pair for each s2 token by reading current state.
	type s2Transfer struct {
		tokenID string
		fromDID string
		toDID   string
	}
	var s2Transfers []s2Transfer
	for _, tid := range s2TokenPool {
		ts, err := w.ReadToken(tid)
		if err != nil {
			fmt.Printf("[STRESS S2] token %s: read error (%v) — skipping\n", tid, err)
			continue
		}
		// Find a toDID different from current owner
		toDID := ""
		for _, d := range dids {
			if d != ts.DID {
				toDID = d
				break
			}
		}
		if toDID == "" {
			fmt.Printf("[STRESS S2] token %s: cannot find distinct toDID — skipping\n", tid)
			continue
		}
		s2Transfers = append(s2Transfers, s2Transfer{tid, ts.DID, toDID})
	}

	if len(s2Transfers) == 0 {
		fmt.Println("[STRESS S2] parallel independent: no eligible tokens — skipped")
	} else {
		fmt.Printf("[STRESS S2] parallel independent: %d goroutines, each on a distinct token\n", len(s2Transfers))

		s2Results := make(chan stressResult, len(s2Transfers))
		var wg2 sync.WaitGroup
		for i, xfer := range s2Transfers {
			wg2.Add(1)
			go func(idx int, tid, fromDID, toDID string) {
				defer wg2.Done()
				defer func() {
					if r := recover(); r != nil {
						s2Results <- stressResult{idx, fmt.Errorf("panic: %v", r)}
					}
				}()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				req, err := buildTransferReq(w, cfg, fromDID, toDID, tid, int64(idx))
				if err != nil {
					s2Results <- stressResult{idx, err}
					return
				}
				err = w.PersistPostConsensus(ctx, req)
				fmt.Printf("[STRESS S2 g%d] token=%s err=%v\n", idx, tid, err)
				s2Results <- stressResult{idx, err}
			}(i, xfer.tokenID, xfer.fromDID, xfer.toDID)
		}
		wg2.Wait()
		close(s2Results)

		for r := range s2Results {
			if r.err != nil {
				log.Fatalf("STRESS FAIL S2: goroutine %d failed: %v", r.id, r.err)
			}
		}
		fmt.Printf("[STRESS S2] parallel independent: all %d succeeded — PASS\n", len(s2Transfers))

		// Post-validate S2
		for _, xfer := range s2Transfers {
			tok, err := w.ReadToken(xfer.tokenID)
			if err != nil {
				log.Fatalf("[STRESS S2] post-validate: ReadToken(%s): %v", xfer.tokenID, err)
			}
			if tok.DID != xfer.toDID {
				log.Fatalf("[STRESS S2] post-validate: token %s owner=%s, expected %s", xfer.tokenID, tok.DID, xfer.toDID)
			}
			chain, err := w.GetTokenChainByTokenID(xfer.tokenID, false)
			if err != nil {
				log.Fatalf("[STRESS S2] post-validate: GetTokenChainByTokenID(%s): %v", xfer.tokenID, err)
			}
			for i, row := range chain {
				if row.Position != int64(i) {
					log.Fatalf("[STRESS S2] post-validate: token %s chain[%d] position=%d, expected %d", xfer.tokenID, i, row.Position, i)
				}
			}
			fmt.Printf("[STRESS S2] post-validate: token %s owner=%s chainLen=%d — OK\n", xfer.tokenID, tok.DID, len(chain))
		}
	}

	// -----------------------------------------------------------------------
	// Scenario 3: Sequential race (2 goroutines, 1 token)
	// -----------------------------------------------------------------------
	if len(didTokens[dids[0]]) < 2 {
		fmt.Println("[STRESS S3] skipped — only 1 token in dids[0]")
	} else {
		tok1 := didTokens[dids[0]][1]
		tok1State, err := w.ReadToken(tok1)
		if err != nil {
			fmt.Printf("[STRESS S3] skipped — cannot read token %s: %v\n", tok1, err)
		} else {
			s3FromDID := tok1State.DID
			s3ToDID := dids[0]
			if s3ToDID == s3FromDID {
				s3ToDID = dids[1]
			}
			if s3ToDID == s3FromDID {
				s3ToDID = dids[2]
			}

			fmt.Printf("[STRESS S3] sequential race: token=%s (owner=%s -> %s)\n", tok1, s3FromDID, s3ToDID)

			s3Results := make(chan stressResult, 2)
			var wg3 sync.WaitGroup

			// Goroutine A: sleep 50ms then build + persist
			wg3.Add(1)
			go func() {
				defer wg3.Done()
				defer func() {
					if r := recover(); r != nil {
						s3Results <- stressResult{0, fmt.Errorf("panic: %v", r)}
					}
				}()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				time.Sleep(50 * time.Millisecond)
				req, err := buildTransferReq(w, cfg, s3FromDID, s3ToDID, tok1, 0)
				if err != nil {
					s3Results <- stressResult{0, err}
					return
				}
				err = w.PersistPostConsensus(ctx, req)
				fmt.Printf("[STRESS S3 A] token=%s err=%v\n", tok1, err)
				s3Results <- stressResult{0, err}
			}()

			// Goroutine B: build + persist immediately (same initial state as A)
			wg3.Add(1)
			go func() {
				defer wg3.Done()
				defer func() {
					if r := recover(); r != nil {
						s3Results <- stressResult{1, fmt.Errorf("panic: %v", r)}
					}
				}()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				req, err := buildTransferReq(w, cfg, s3FromDID, s3ToDID, tok1, 1)
				if err != nil {
					s3Results <- stressResult{1, err}
					return
				}
				err = w.PersistPostConsensus(ctx, req)
				fmt.Printf("[STRESS S3 B] token=%s err=%v\n", tok1, err)
				s3Results <- stressResult{1, err}
			}()

			wg3.Wait()
			close(s3Results)

			var s3Succeeded int
			for r := range s3Results {
				if r.err == nil {
					s3Succeeded++
				}
			}
			if s3Succeeded != 1 {
				log.Fatalf("STRESS FAIL S3: expected 1 success, got %d", s3Succeeded)
			}
			fmt.Printf("[STRESS S3] sequential race: 1 success, 1 failed — PASS\n")
		}
	}

	// -----------------------------------------------------------------------
	// Build availableTokens and currentOwners for fuzz tests
	// -----------------------------------------------------------------------
	// Tokens consumed by concurrency scenarios:
	// S1 consumed: didTokens[dids[0]][0]
	// S3 consumed: didTokens[dids[0]][1] (if existed and 1 goroutine succeeded)
	// S2 consumed: s2TokenPool tokens
	usedSet := map[string]bool{}
	usedSet[didTokens[dids[0]][0]] = true
	if len(didTokens[dids[0]]) >= 2 {
		usedSet[didTokens[dids[0]][1]] = true
	}
	// Also mark all s2 token pool tokens as used
	for _, tid := range s2TokenPool {
		usedSet[tid] = true
	}

	// Collect all known tokens from the original didTokens map
	var allKnown []string
	for _, toks := range didTokens {
		allKnown = append(allKnown, toks...)
	}

	var availableTokens []string
	currentOwners := make(map[string]string)

	for _, tokenID := range allKnown {
		tok, err := w.ReadToken(tokenID)
		if err == nil {
			currentOwners[tokenID] = tok.DID
			if !usedSet[tokenID] {
				availableTokens = append(availableTokens, tokenID)
			}
		}
	}

	fmt.Println("\n=== FUZZ TESTS ===")
	runTransferFuzz(w, cfg, dids, availableTokens, currentOwners)

	fmt.Println("\n=== POST-HARNESS ASSERTIONS ===")
	for _, tokenID := range allKnown {
		tok, err := w.ReadToken(tokenID)
		if err != nil {
			fmt.Printf("[HARNESS] token %s: read error (may be expected) — skipping\n", tokenID)
			continue
		}
		if owner, ok := currentOwners[tokenID]; ok && tok.DID != owner {
			log.Fatalf("[HARNESS] FAIL: token %s owner mismatch: DB=%s expected=%s", tokenID, tok.DID, owner)
		}
		chain, err := w.GetTokenChainByTokenID(tokenID, false)
		if err != nil {
			log.Fatalf("[HARNESS] GetTokenChainByTokenID(%s): %v", tokenID, err)
		}
		for i, row := range chain {
			if row.Position != int64(i) {
				log.Fatalf("[HARNESS] FAIL: token %s chain[%d] position=%d, expected %d", tokenID, i, row.Position, i)
			}
		}
		for i := 1; i < len(chain); i++ {
			if chain[i].PreviousTransactionID == nil || *chain[i].PreviousTransactionID != chain[i-1].TransactionID {
				log.Fatalf("[HARNESS] FAIL: token %s chain[%d] prev_tx linkage broken", tokenID, i)
			}
		}
		if chain[0].PreviousTransactionID != nil && *chain[0].PreviousTransactionID != "" {
			log.Fatalf("[HARNESS] FAIL: token %s genesis row has non-nil PreviousTransactionID", tokenID)
		}
		lastChain := chain[len(chain)-1]
		if tok.TransactionID != lastChain.TransactionID {
			log.Fatalf("[HARNESS] FAIL: token %s tx mismatch: token.TransactionID=%s chain.last=%s", tokenID, tok.TransactionID, lastChain.TransactionID)
		}
	}
	fmt.Println("[HARNESS] post-harness assertions PASS")
	fmt.Println("\n=== ALL STRESS + FUZZ TESTS PASSED ===")
}

// runTransferFuzz runs 20 fuzz cases against PersistPostConsensus:
//   - Cases 1-10: adversarial (MUST fail with a non-nil error)
//   - Cases 11-20: valid transfers (MUST succeed; updates currentOwners on success)
func runTransferFuzz(w *wallet.Wallet, cfg *types.RubixConfig, dids []string, availableTokens []string, currentOwners map[string]string) {
	// Find a stable token for adversarial cases. All adversarial cases fail before any DB write,
	// so the same token can be reused across cases 1-10.
	advTok := ""
	advFromDID := ""
	for _, tid := range availableTokens {
		if owner, ok := currentOwners[tid]; ok && owner != "" {
			advTok = tid
			advFromDID = owner
			break
		}
	}
	if advTok == "" {
		fmt.Println("[FUZZ] no available token for adversarial cases — skipping adversarial fuzz")
	}

	// ---- Case 1: Invalid owner (wrong DID in req.DID) ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 1] ReadToken(%s): %v", advTok, err)
		}
		req, _, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 0)
		if err != nil {
			log.Fatalf("[FUZZ case 1] buildTransferReqFromTok: %v", err)
		}
		// Set req.DID to a wrong DID (not the current owner)
		wrongDID := ""
		for _, d := range dids {
			if d != advFromDID {
				wrongDID = d
				break
			}
		}
		req.DID = wrongDID
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 1 (invalid owner) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 1] invalid owner → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 1] invalid owner — skipped (no token)")
	}

	// ---- Case 2: Wrong prev_tx_id ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 2] ReadToken(%s): %v", advTok, err)
		}
		req, txInfo, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 1)
		if err != nil {
			log.Fatalf("[FUZZ case 2] buildTransferReqFromTok: %v", err)
		}
		wrongPrev := "deadbeef-wrong"
		req.TokenChainRows[0].PreviousTransactionID = ptrStr(wrongPrev)
		// Also update txInfo.Tokens.RBT[0].PreviousTransactionID to match
		if txInfo.Tokens != nil && len(txInfo.Tokens.RBT) > 0 {
			txInfo.Tokens.RBT[0].PreviousTransactionID = wrongPrev
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 2 (wrong prev_tx_id) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 2] wrong prev_tx_id → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 2] wrong prev_tx_id — skipped (no token)")
	}

	// ---- Case 3: Wrong position ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 3] ReadToken(%s): %v", advTok, err)
		}
		req, _, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 2)
		if err != nil {
			log.Fatalf("[FUZZ case 3] buildTransferReqFromTok: %v", err)
		}
		badPos := advTokState.LatestPosition + 99
		req.TokenChainRows[0].Position = badPos
		req.TokenStates[0].LatestPosition = badPos
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 3 (wrong position) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 3] wrong position → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 3] wrong position — skipped (no token)")
	}

	// ---- Case 4: Empty signature ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 4] ReadToken(%s): %v", advTok, err)
		}
		req, _, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 3)
		if err != nil {
			log.Fatalf("[FUZZ case 4] buildTransferReqFromTok: %v", err)
		}
		req.Signature = &models.Signature{InitiatorSignature: ""}
		emptySigBytes, _ := json.Marshal(req.Signature)
		req.Transaction.Signature = json.RawMessage(emptySigBytes)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 4 (empty signature) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 4] empty signature → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 4] empty signature — skipped (no token)")
	}

	// ---- Case 5: Corrupted sig (last 4 chars replaced with "ZZZZ") ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 5] ReadToken(%s): %v", advTok, err)
		}
		req, _, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 4)
		if err != nil {
			log.Fatalf("[FUZZ case 5] buildTransferReqFromTok: %v", err)
		}
		sigHex := req.Signature.InitiatorSignature
		if len(sigHex) >= 4 {
			sigHex = sigHex[:len(sigHex)-4] + "ZZZZ"
		}
		req.Signature.InitiatorSignature = sigHex
		corruptedSigBytes, _ := json.Marshal(req.Signature)
		req.Transaction.Signature = json.RawMessage(corruptedSigBytes)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 5 (corrupted sig) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 5] corrupted sig → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 5] corrupted sig — skipped (no token)")
	}

	// ---- Case 6: Sig reuse (sign txInfo_A, use with txInfo_B) ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 6] ReadToken(%s): %v", advTok, err)
		}
		epochA := time.Now().UnixNano()
		epochB := epochA + 1

		txInfoA := &models.TransactionInfo{
			Initiator: advFromDID,
			Owner:     dids[1],
			Epoch:     int(epochA),
			Network:   "local",
			Tokens: &models.TransactionTokens{
				RBT: []*models.TokenInfo{
					{
						TokenID:               advTok,
						PreviousTransactionID: advTokState.TransactionID,
						TokenValue:            advTokState.TokenValue,
					},
				},
			},
		}
		dcA := did.InitDIDLiteWithPassword(advFromDID, cfg.DidDir, "pwd-1")
		sigHexA, err := util.SignTransaction(dcA, txInfoA)
		if err != nil {
			log.Fatalf("[FUZZ case 6] SignTransaction txInfoA: %v", err)
		}

		txInfoB := &models.TransactionInfo{
			Initiator: advFromDID,
			Owner:     dids[2],
			Epoch:     int(epochB),
			Network:   "local",
			Tokens: &models.TransactionTokens{
				RBT: []*models.TokenInfo{
					{
						TokenID:               advTok,
						PreviousTransactionID: advTokState.TransactionID,
						TokenValue:            advTokState.TokenValue,
					},
				},
			},
		}
		txIDB, err := util.GetTransactionID(txInfoB)
		if err != nil {
			log.Fatalf("[FUZZ case 6] ComputeTransactionID txInfoB: %v", err)
		}
		infoBytesB, err := models.SerializeTransactionInfo(txInfoB)
		if err != nil {
			log.Fatalf("[FUZZ case 6] SerializeTransactionInfo txInfoB: %v", err)
		}
		// Sig from txInfoA reused for txInfoB
		sigStructReuse := &models.Signature{InitiatorSignature: sigHexA}
		sigBytesReuse, _ := json.Marshal(sigStructReuse)

		newPos := advTokState.LatestPosition + 1
		prevTxID := advTokState.TransactionID
		transferRole := int16(models.GetTokenRoleID(constants.TokenRole_Transfer))

		req := &wallet.PostConsensusPersistenceRequest{
			Transaction: &models.Transactions{
				ID:        txIDB,
				Info:      infoBytesB,
				Signature: json.RawMessage(sigBytesReuse),
			},
			TransactionInfo: txInfoB, // txInfoB but sig from txInfoA
			Signature:       sigStructReuse,
			DID:             advFromDID,
			ExecutionRole:   wallet.ExecutionRoleInitiator,
			AffectedTokens:  []string{advTok},
			TokenChainRows: []models.TokenChain{
				{
					TokenID:               advTok,
					TransactionID:         txIDB,
					PreviousTransactionID: &prevTxID,
					Role:                  transferRole,
					Position:              newPos,
				},
			},
			TokenStates: []models.Token{
				{
					TokenID:        advTok,
					DID:            dids[2],
					TransactionID:  txIDB,
					TokenValue:     advTokState.TokenValue,
					TokenStatus:    int16(constants.TokenStatus_Free),
					TokenType:      advTokState.TokenType,
					LatestPosition: newPos,
					LatestRole:     transferRole,
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 6 (sig reuse) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 6] sig reuse → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 6] sig reuse — skipped (no token)")
	}

	// ---- Case 7: Modified txInfo after signing (Owner tampered) ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 7] ReadToken(%s): %v", advTok, err)
		}
		req, _, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 5)
		if err != nil {
			log.Fatalf("[FUZZ case 7] buildTransferReqFromTok: %v", err)
		}
		// Mutate txInfo.Owner after signing — sig is still the original, bound to original Owner
		req.TransactionInfo.Owner = "tampered-did"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 7 (modified txInfo after signing) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 7] modified txInfo after signing → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 7] modified txInfo after signing — skipped (no token)")
	}

	// ---- Case 8: Duplicate tokenID in AffectedTokens ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 8] ReadToken(%s): %v", advTok, err)
		}
		req, _, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 6)
		if err != nil {
			log.Fatalf("[FUZZ case 8] buildTransferReqFromTok: %v", err)
		}
		req.AffectedTokens = []string{advTok, advTok}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 8 (duplicate tokenID in AffectedTokens) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 8] duplicate tokenID in AffectedTokens → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 8] duplicate tokenID in AffectedTokens — skipped (no token)")
	}

	// ---- Case 9: Empty tokenID in TokenChainRows ----
	if advTok != "" {
		advTokState, err := w.ReadToken(advTok)
		if err != nil {
			log.Fatalf("[FUZZ case 9] ReadToken(%s): %v", advTok, err)
		}
		req, _, _, err := buildTransferReqFromTok(cfg, advFromDID, dids[1], advTokState, 7)
		if err != nil {
			log.Fatalf("[FUZZ case 9] buildTransferReqFromTok: %v", err)
		}
		req.TokenChainRows[0].TokenID = ""
		req.TokenStates[0].TokenID = ""
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, req)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 9 (empty tokenID in TokenChainRows) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 9] empty tokenID in TokenChainRows → EXPECTED FAIL → FAIL: %v\n", err)
	} else {
		fmt.Println("[FUZZ case 9] empty tokenID in TokenChainRows — skipped (no token)")
	}

	// ---- Case 10: Non-existent token ----
	{
		// Build a plausible-looking req for a token that does not exist in DB.
		// We cannot call ReadToken since the token doesn't exist, so we construct manually.
		fakeTokenID := "99_99999"
		fakePrevTxID := "fake"
		fakePos := int64(1) // > 0 so validateTransferChainContinuity processes it (not genesis skip)
		transferRole := int16(models.GetTokenRoleID(constants.TokenRole_Transfer))

		txInfoFake := &models.TransactionInfo{
			Initiator: dids[0],
			Owner:     dids[1],
			Epoch:     int(time.Now().UnixNano()),
			Network:   "local",
			Tokens: &models.TransactionTokens{
				RBT: []*models.TokenInfo{
					{
						TokenID:               fakeTokenID,
						PreviousTransactionID: fakePrevTxID,
						TokenValue:            1.0,
					},
				},
			},
		}
		dcFake := did.InitDIDLiteWithPassword(dids[0], cfg.DidDir, "pwd-1")
		sigHexFake, err := util.SignTransaction(dcFake, txInfoFake)
		if err != nil {
			log.Fatalf("[FUZZ case 10] SignTransaction: %v", err)
		}
		infoByteFake, err := models.SerializeTransactionInfo(txInfoFake)
		if err != nil {
			log.Fatalf("[FUZZ case 10] SerializeTransactionInfo: %v", err)
		}
		txIDFake, err := util.GetTransactionID(txInfoFake)
		if err != nil {
			log.Fatalf("[FUZZ case 10] ComputeTransactionID: %v", err)
		}
		sigStructFake := &models.Signature{InitiatorSignature: sigHexFake}
		sigBytesFake, _ := json.Marshal(sigStructFake)

		reqFake := &wallet.PostConsensusPersistenceRequest{
			Transaction: &models.Transactions{
				ID:        txIDFake,
				Info:      infoByteFake,
				Signature: json.RawMessage(sigBytesFake),
			},
			TransactionInfo: txInfoFake,
			Signature:       sigStructFake,
			DID:             dids[0],
			ExecutionRole:   wallet.ExecutionRoleInitiator,
			AffectedTokens:  []string{fakeTokenID},
			TokenChainRows: []models.TokenChain{
				{
					TokenID:               fakeTokenID,
					TransactionID:         txIDFake,
					PreviousTransactionID: &fakePrevTxID,
					Role:                  transferRole,
					Position:              fakePos,
				},
			},
			TokenStates: []models.Token{
				{
					TokenID:        fakeTokenID,
					DID:            dids[1],
					TransactionID:  txIDFake,
					TokenValue:     1.0,
					TokenStatus:    int16(constants.TokenStatus_Free),
					TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
					LatestPosition: fakePos,
					LatestRole:     transferRole,
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = w.PersistPostConsensus(ctx, reqFake)
		cancel()
		if err == nil {
			log.Fatalf("FUZZ FAIL: case 10 (non-existent token) should have failed but succeeded")
		}
		fmt.Printf("[FUZZ case 10] non-existent token → EXPECTED FAIL → FAIL: %v\n", err)
	}

	// ---- Cases 11-20: Valid transfers ----
	validIdx := 0
	for caseNum := 11; caseNum <= 20; caseNum++ {
		if validIdx >= len(availableTokens) {
			fmt.Printf("[FUZZ case %d] valid transfer — skipped (no tokens left)\n", caseNum)
			continue
		}
		tok := availableTokens[validIdx]
		validIdx++

		fromDID, ok := currentOwners[tok]
		if !ok || fromDID == "" {
			fmt.Printf("[FUZZ case %d] valid transfer — skipped (no owner for token %s)\n", caseNum, tok)
			continue
		}

		// Find index of fromDID in dids slice
		fromIdx := -1
		for i, d := range dids {
			if d == fromDID {
				fromIdx = i
				break
			}
		}
		var toDID string
		if fromIdx >= 0 {
			toDID = dids[(fromIdx+1)%len(dids)]
		} else {
			// fromDID not in dids (e.g. it was a non-original DID) — pick dids[0]
			toDID = dids[0]
			if toDID == fromDID {
				toDID = dids[1]
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := func() error {
			req, err := buildTransferReq(w, cfg, fromDID, toDID, tok, int64(caseNum))
			if err != nil {
				return err
			}
			return w.PersistPostConsensus(ctx, req)
		}()
		cancel()

		if err != nil {
			log.Fatalf("FUZZ FAIL: case %d should have succeeded: %v", caseNum, err)
		}
		fmt.Printf("[FUZZ case %d] valid transfer %s -> %s — PASS\n", caseNum, fromDID, toDID)
		currentOwners[tok] = toDID
	}
}
