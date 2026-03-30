package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func main_old() {
	// Step 1: Parse config from ./config directory
	userCfg, err := config.ParseConfigFromPath("./config")
	if err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}

	// Step 2: Create RubixConfig from UserConfig, using "." as nodeDir
	cfg, err := config.CreateRubixConfigFromUserConfig(userCfg, ".")
	if err != nil {
		log.Fatalf("failed to create rubix config: %v", err)
	}

	// Fix: CfgData.Ports is never populated by CreateRubixConfigFromUserConfig.
	// The IPFS health manager reads cfg.CfgData.Ports.IPFSPort (defaults to 0),
	// causing health checks to ping port 0 and all IPFSOperations methods timeout.
	cfg.CfgData.Ports = cfg.PortConfig

	// Fail fast if IPFS binary not available
	if _, err := os.Stat("./ipfs"); os.IsNotExist(err) {
		log.Fatalf("IPFS binary not found at ./ipfs -- run from project root")
	}

	// Step 3: Create logger
	lg := logger.New(&logger.LoggerOptions{Name: "dev"})

	// Step 4: Call NewCore -- note &cfg (address-of value)
	c, err := core.NewCore(&cfg, lg, "localnet", false, false, false, "")
	if err != nil {
		log.Fatalf("failed to create core: %v", err)
	}

	fmt.Println("Core initialized successfully.")

	// --- IPFS Lifecycle ---
	if err := c.RunIPFS(); err != nil {
		log.Fatalf("RunIPFS failed: %v", err)
	}
	defer c.StopCore()
	fmt.Println("IPFS daemon started successfully.")

	// IPFS assertions
	if !c.GetIPFSState() {
		log.Fatalf("ASSERTION FAILED: GetIPFSState() returned false after RunIPFS")
	}
	peerID := c.GetPeerID()
	if peerID == "" {
		log.Fatalf("ASSERTION FAILED: GetPeerID() returned empty string after RunIPFS")
	}
	fmt.Printf("IPFS PeerID: %s\n", peerID)

	if c.IPFSOperations() == nil {
		log.Fatalf("ASSERTION FAILED: IPFSOperations() returned nil after RunIPFS")
	}

	// IPFS write test: AddDir with a temp file
	ipfsTestDir, err := os.MkdirTemp("", "ipfs-dev-test-*")
	if err != nil {
		log.Fatalf("failed to create temp dir for IPFS test: %v", err)
	}
	defer os.RemoveAll(ipfsTestDir)

	testFilePath := filepath.Join(ipfsTestDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("rubix dev runner IPFS test"), 0644); err != nil {
		log.Fatalf("failed to write IPFS test file: %v", err)
	}

	cid, err := c.IPFSOperations().AddDir(ipfsTestDir)
	if err != nil {
		log.Fatalf("IPFSOperations().AddDir failed: %v", err)
	}
	if cid == "" {
		log.Fatalf("ASSERTION FAILED: AddDir returned empty CID")
	}
	fmt.Printf("IPFS AddDir CID: %s\n", cid)
	fmt.Println("IPFS integration validated.")

	// --- DID Module Initialization ---
	c.InitDIDModule()
	fmt.Println("DID module initialized.")

	// Step A: Get wallet reference
	w := c.GetWallet()

	// Step B: Create sender DID (real keypair + IPFS CID + Postgres upsert)
	senderDID, err := c.CreateDID(&did.DIDCreate{PrivPWD: "test-sender-pwd"}, true)
	if err != nil {
		log.Fatalf("CreateDID(sender) failed: %v", err)
	}
	fmt.Printf("Sender DID created: %s\n", senderDID)

	// Step B2: Verify sender DID artifacts on disk
	didDir := cfg.DidDir

	// Create DIDCrypto instance for sender (used for genesis signing and transfer signing)
	senderDC := did.InitDIDLiteWithPassword(senderDID, didDir, "test-sender-pwd")
	for _, fname := range []string{did.PvtKeyFileName, did.PubKeyFileName, did.MnemonicFileName} {
		fpath := filepath.Join(didDir, senderDID, fname)
		info, err := os.Stat(fpath)
		if err != nil {
			log.Fatalf("ASSERTION FAILED: DID artifact missing: %s (%v)", fpath, err)
		}
		if info.Size() == 0 {
			log.Fatalf("ASSERTION FAILED: DID artifact empty: %s", fpath)
		}
		fmt.Printf("DID artifact OK: %s (%d bytes)\n", fname, info.Size())
	}

	// Step C: Seed 3 genesis RBT tokens (idempotent -- check existence first)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		currentTime := int(time.Now().Unix())

		tx, err := w.BeginTx(ctx)
		if err != nil {
			fmt.Printf("PersistGenesisTokenRecord: begin tx: %w", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		tokenID := fmt.Sprintf("QmTestToken%03d", i)

		existing, readErr := w.ReadToken(tokenID)
		if readErr == nil && existing != nil {
			fmt.Printf("Token %s already exists, skipping.\n", tokenID)
			continue
		}

		txID, err := w.PersistGenesisTokenRecord(tx, senderDC, nil, tokenID, senderDID, constants.NetworkMode_Localnet, currentTime)
		if err != nil {
			log.Fatalf("PersistGenesisTokenRecord failed for %s: %v", tokenID, err)
		}
		fmt.Printf("Token %s seeded successfully (txID: %s).\n", tokenID, txID)
	}

	// Step C2: Verify tokenchain_index exists for each genesis token (1 entry each)
	fmt.Println("\n--- Step C2: tokenchain_index verification after genesis ---")
	for i := 1; i <= 3; i++ {
		tokenID := fmt.Sprintf("QmTestToken%03d", i)
		idx, err := w.GetTokenchainIndex(tokenID)
		if err != nil {
			log.Fatalf("GetTokenchainIndex failed for %s: %v", tokenID, err)
		}
		if idx == nil {
			log.Fatalf("ASSERTION FAILED: tokenchain_index is nil for genesis token %s", tokenID)
		}
		if len(idx.Index) != 1 {
			log.Fatalf("ASSERTION FAILED: expected 1 tokenchain_index entry for %s, got %d", tokenID, len(idx.Index))
		}
		fmt.Printf("tokenchain_index for %s: %v\n", tokenID, idx.Index)
	}

	// Step D: Verify by reading back free tokens for the sender DID
	tokens, tokenIDs, err := w.GetFreeRBTTokens(senderDID)
	if err != nil {
		log.Fatalf("GetFreeRBTTokens failed: %v", err)
	}
	fmt.Printf("Free RBT tokens for %s: count=%d, ids=%v\n", senderDID, len(tokens), tokenIDs)

	// Step E: Verify individual token read-back
	for i := 1; i <= 3; i++ {
		tokenID := fmt.Sprintf("QmTestToken%03d", i)
		t, err := w.ReadToken(tokenID)
		if err != nil {
			log.Fatalf("ReadToken failed for %s: %v", tokenID, err)
		}
		fmt.Printf("ReadToken %s: TokenID=%s TokenValue=%v TokenStatus=%d DID=%s\n",
			tokenID, t.TokenID, t.TokenValue, t.TokenStatus, t.DID)
	}

	// Step F: Simulate transfer via PersistPostConsensus
	fmt.Println("\n--- Step F: Transfer simulation via PersistPostConsensus ---")

	// F1: Read QmTestToken001 to get its current TransactionID (needed as PreviousTransactionID)
	preToken, err := w.ReadToken("QmTestToken001")
	if err != nil {
		log.Fatalf("ReadToken(QmTestToken001) failed before transfer: %v", err)
	}
	fmt.Printf("Pre-transfer state: TokenID=%s TransactionID=%s LatestPosition=%d LatestRole=%d\n",
		preToken.TokenID, preToken.TransactionID, preToken.LatestPosition, preToken.LatestRole)

	// F2: Build TransactionInfo for transfer (epoch=1)
	transferTxInfo := &models.TransactionInfo{
		Initiator: senderDID,
		Owner:     senderDID, // same DID for simplicity (initiator perspective)
		Epoch:     1,
		Network:   constants.NetworkID_RBT_Local,
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{
					TokenID:               "QmTestToken001",
					PreviousTransactionID: preToken.TransactionID,
					TokenValue:            1.0,
					DID:                   senderDID,
				},
			},
		},
	}

	// F3: Compute transaction ID for logging
	transferTxID, err := util.GetTransactionID(transferTxInfo)
	if err != nil {
		log.Fatalf("ComputeTransactionID for transfer failed: %v", err)
	}
	fmt.Printf("Transfer transaction ID: %s\n", transferTxID)

	// F4: Pattern A -- sign with sender's private key, verify before persistence
	initiatorSigF, err := util.SignTransaction(senderDC, transferTxInfo)
	if err != nil {
		log.Fatalf("SignTransaction(step F) failed: %v", err)
	}
	if err := util.VerifySignature(senderDC, transferTxInfo, initiatorSigF); err != nil {
		log.Fatalf("VerifySignature(step F) failed: %v", err)
	}
	fmt.Println("Step F: signature verified")

	transferSig := &models.Signature{
		InitiatorSignature: initiatorSigF,
		Quorums:            nil,
	}

	if err := w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
		TransactionInfo: transferTxInfo,
		Signature:       transferSig,
		DID:             senderDID,
		ExecutionRole:   wallet.ExecutionRoleInitiator,
	}); err != nil {
		log.Fatalf("PersistPostConsensus failed: %v", err)
	}
	fmt.Println("PostConsensus complete")

	// F5: Verify -- read back token after transfer
	postToken, err := w.ReadToken("QmTestToken001")
	if err != nil {
		log.Fatalf("ReadToken(QmTestToken001) failed after transfer: %v", err)
	}
	fmt.Printf("Post-transfer state: TokenID=%s TransactionID=%s LatestPosition=%d LatestRole=%d DID=%s\n",
		postToken.TokenID, postToken.TransactionID, postToken.LatestPosition, postToken.LatestRole, postToken.DID)

	// Assert expected values
	if postToken.LatestPosition != 1 {
		log.Fatalf("ASSERTION FAILED: expected LatestPosition=1, got %d", postToken.LatestPosition)
	}
	if postToken.LatestRole != 2 { // 2 = transfer role
		log.Fatalf("ASSERTION FAILED: expected LatestRole=2 (transfer), got %d", postToken.LatestRole)
	}
	if postToken.TransactionID != transferTxID {
		log.Fatalf("ASSERTION FAILED: expected TransactionID=%s, got %s", transferTxID, postToken.TransactionID)
	}

	// F6: Verify -- tokenchain shows position 0 (mint) + position 1 (transfer)
	chain, err := w.GetTokenChainByTokenID("QmTestToken001")
	if err != nil {
		log.Fatalf("GetTokenChainByTokenID(QmTestToken001) failed: %v", err)
	}
	fmt.Printf("Tokenchain entries for QmTestToken001: %d\n", len(chain))
	for _, row := range chain {
		prevTxID := "<nil>"
		if row.PreviousTransactionID != nil {
			prevTxID = *row.PreviousTransactionID
		}
		fmt.Printf("  position=%d role=%d txID=%s prevTxID=%s\n",
			row.Position, row.Role, row.TransactionID, prevTxID)
	}
	if len(chain) != 2 {
		log.Fatalf("ASSERTION FAILED: expected 2 tokenchain entries, got %d", len(chain))
	}
	if chain[0].Position != 0 || chain[1].Position != 1 {
		log.Fatalf("ASSERTION FAILED: expected positions [0,1], got [%d,%d]", chain[0].Position, chain[1].Position)
	}

	// F6b: Verify tokenchain_index for QmTestToken001 after transfer (genesis + transfer = 2 entries)
	fmt.Println("\n--- Step F6b: tokenchain_index verification after transfer ---")
	transferIdx, err := w.GetTokenchainIndex("QmTestToken001")
	if err != nil {
		log.Fatalf("GetTokenchainIndex(QmTestToken001) failed after transfer: %v", err)
	}
	if transferIdx == nil {
		log.Fatalf("ASSERTION FAILED: tokenchain_index is nil for QmTestToken001 after transfer")
	}
	if len(transferIdx.Index) != 2 {
		log.Fatalf("ASSERTION FAILED: expected 2 tokenchain_index entries for QmTestToken001, got %d", len(transferIdx.Index))
	}
	fmt.Printf("tokenchain_index for QmTestToken001: %v\n", transferIdx.Index)

	// F7: Verify -- transfer transaction exists in transactions table
	txRecord, err := w.GetTransactionByID(transferTxID)
	if err != nil {
		log.Fatalf("GetTransactionByID(%s) failed: %v", transferTxID, err)
	}
	if txRecord == nil {
		log.Fatalf("ASSERTION FAILED: transfer transaction %s not found in DB", transferTxID)
	}
	fmt.Printf("Transfer transaction found: ID=%s\n", txRecord.ID)

	fmt.Printf("\nDev runner complete: DID %s + 3 tokens seeded + transfer simulated and verified.\n", senderDID)

	// -----------------------------------------------------------------------
	// Steps G-K: Full 2-DID transaction lifecycle
	// -----------------------------------------------------------------------

	// Step G: Create receiver DID (real keypair + IPFS CID + Postgres upsert)
	fmt.Println("\n--- Step G: Create receiver DID ---")
	receiverDID, err := c.CreateDID(&did.DIDCreate{PrivPWD: "test-receiver-pwd"}, true)
	if err != nil {
		log.Fatalf("CreateDID(receiver) failed: %v", err)
	}
	fmt.Printf("Receiver DID created: %s\n", receiverDID)

	// Create DIDCrypto instance for receiver
	receiverDC := did.InitDIDLiteWithPassword(receiverDID, didDir, "test-receiver-pwd")
	_ = receiverDC // used in future tasks

	// Step G2: Verify receiver DID artifacts on disk
	for _, fname := range []string{did.PvtKeyFileName, did.PubKeyFileName, did.MnemonicFileName} {
		fpath := filepath.Join(didDir, receiverDID, fname)
		info, err := os.Stat(fpath)
		if err != nil {
			log.Fatalf("ASSERTION FAILED: Receiver DID artifact missing: %s (%v)", fpath, err)
		}
		if info.Size() == 0 {
			log.Fatalf("ASSERTION FAILED: Receiver DID artifact empty: %s", fpath)
		}
		fmt.Printf("Receiver DID artifact OK: %s (%d bytes)\n", fname, info.Size())
	}

	// Step H: Transfer all 3 tokens from senderDID to receiverDID
	fmt.Println("\n--- Step H: Transfer 3 tokens from sender to receiver ---")
	allTokenIDs := []string{"QmTestToken001", "QmTestToken002", "QmTestToken003"}

	// Track genesis txID for QmTestToken001 (needed for Step J PreviousTransactionID assertion)
	// After step F, QmTestToken001's TransactionID is the step-F transfer txID.
	// We need the current token state before step H for each token.
	stepHTransferTxIDs := make(map[string]string)

	for _, tid := range allTokenIDs {
		// H1: Read current token state to get current TransactionID (used as PreviousTransactionID)
		preToken, err := w.ReadToken(tid)
		if err != nil {
			log.Fatalf("ReadToken(%s) failed before step-H transfer: %v", tid, err)
		}
		fmt.Printf("Pre-H-transfer: TokenID=%s TransactionID=%s LatestPosition=%d DID=%s\n",
			preToken.TokenID, preToken.TransactionID, preToken.LatestPosition, preToken.DID)

		// H2: Build TransactionInfo for transfer
		// Epoch=2 because step F already used epoch=1 for QmTestToken001;
		// using epoch=2 here keeps txIDs distinct for all 3 tokens in this batch.
		transferTxInfoH := &models.TransactionInfo{
			Initiator: senderDID,
			Owner:     receiverDID,
			Epoch:     2,
			Network:   constants.NetworkID_RBT_Local,
			Tokens: &models.TransactionTokens{
				RBT: []*models.TokenInfo{
					{
						TokenID:               tid,
						PreviousTransactionID: preToken.TransactionID,
						TokenValue:            1.0,
						DID:                   senderDID,
					},
				},
			},
		}

		// H3: Compute transaction ID
		txIDH, err := util.GetTransactionID(transferTxInfoH)
		if err != nil {
			log.Fatalf("ComputeTransactionID failed for %s (step H): %v", tid, err)
		}
		stepHTransferTxIDs[tid] = txIDH

		// H4: Pattern A -- sign with sender's private key, verify before persistence
		initiatorSigH, err := util.SignTransaction(senderDC, transferTxInfoH)
		if err != nil {
			log.Fatalf("SignTransaction(step H, %s) failed: %v", tid, err)
		}
		if err := util.VerifySignature(senderDC, transferTxInfoH, initiatorSigH); err != nil {
			log.Fatalf("VerifySignature(step H, %s) failed: %v", tid, err)
		}
		fmt.Printf("Step H: signature verified for %s\n", tid)

		sigH := &models.Signature{
			InitiatorSignature: initiatorSigH,
			Quorums:            nil,
		}

		// H5: Initiator-side persistence (token.DID stays senderDID after this call)
		/*
			if err := w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
				TransactionInfo: transferTxInfoH,
				Signature:       sigH,
				DID:             senderDID,
				ExecutionRole:   wallet.ExecutionRoleInitiator,
			}); err != nil {
				log.Fatalf("PersistPostConsensus(initiator) failed for %s: %v", tid, err)
			}
		*/

		// H6: Receiver-side persistence (token.DID becomes receiverDID after this call)
		// The tokenchain INSERT is ON CONFLICT DO NOTHING -- only the token.DID upsert changes.
		if err := w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
			TransactionInfo: transferTxInfoH,
			Signature:       sigH,
			DID:             receiverDID,
			ExecutionRole:   wallet.ExecutionRoleReceiver,
		}); err != nil {
			log.Fatalf("PersistPostConsensus(receiver) failed for %s: %v", tid, err)
		}

		fmt.Printf("Step H transfer complete for %s: txID=%s\n", tid, txIDH)
	}

	// Step I: Ownership assertions
	fmt.Println("\n--- Step I: Ownership assertions ---")

	// I1: Each token must now be owned by receiverDID
	for _, tid := range allTokenIDs {
		tok, err := w.ReadToken(tid)
		if err != nil {
			log.Fatalf("ReadToken(%s) failed in step I: %v", tid, err)
		}
		if tok.DID != receiverDID {
			log.Fatalf("ASSERTION FAILED: expected token %s DID=%s, got %s", tid, receiverDID, tok.DID)
		}
		fmt.Printf("Ownership OK: %s.DID = %s\n", tid, tok.DID)
	}

	// I2: senderDID must have 0 free tokens
	senderTokens, senderTokenIDs, err := w.GetFreeRBTTokens(senderDID)
	if err != nil {
		log.Fatalf("GetFreeRBTTokens(senderDID) failed: %v", err)
	}
	if len(senderTokens) != 0 {
		log.Fatalf("ASSERTION FAILED: expected 0 free tokens for sender, got %d (ids: %v)", len(senderTokens), senderTokenIDs)
	}
	fmt.Printf("Sender free tokens: %d (expected 0) -- OK\n", len(senderTokens))

	// I3: receiverDID must have 3 free tokens
	receiverTokens, receiverTokenIDList, err := w.GetFreeRBTTokens(receiverDID)
	if err != nil {
		log.Fatalf("GetFreeRBTTokens(receiverDID) failed: %v", err)
	}
	if len(receiverTokens) != 3 {
		log.Fatalf("ASSERTION FAILED: expected 3 free tokens for receiver, got %d (ids: %v)", len(receiverTokens), receiverTokenIDList)
	}
	fmt.Printf("Receiver free tokens: %d (expected 3), ids=%v -- OK\n", len(receiverTokens), receiverTokenIDList)

	// Step J: Tokenchain integrity assertions
	fmt.Println("\n--- Step J: Tokenchain integrity assertions ---")

	// Expected chain lengths:
	// QmTestToken001: 3 entries (genesis + step-F transfer + step-H transfer)
	// QmTestToken002, QmTestToken003: 2 entries (genesis + step-H transfer)
	expectedChainLen := map[string]int{
		"QmTestToken001": 3,
		"QmTestToken002": 2,
		"QmTestToken003": 2,
	}

	for _, tid := range allTokenIDs {
		chainEntries, err := w.GetTokenChainByTokenID(tid)
		if err != nil {
			log.Fatalf("GetTokenChainByTokenID(%s) failed: %v", tid, err)
		}

		expected := expectedChainLen[tid]
		fmt.Printf("Tokenchain for %s (%d entries):\n", tid, len(chainEntries))
		for _, row := range chainEntries {
			prevTxStr := "<nil>"
			if row.PreviousTransactionID != nil {
				prevTxStr = *row.PreviousTransactionID
			}
			fmt.Printf("  position=%d role=%d txID=%s prevTxID=%s\n",
				row.Position, row.Role, row.TransactionID, prevTxStr)
		}

		if len(chainEntries) != expected {
			log.Fatalf("ASSERTION FAILED: %s expected %d tokenchain entries, got %d", tid, expected, len(chainEntries))
		}

		// Assert positions are contiguous from 0
		for i, row := range chainEntries {
			if row.Position != int64(i) {
				log.Fatalf("ASSERTION FAILED: %s position[%d] expected %d, got %d", tid, i, i, row.Position)
			}
		}

		// Assert position 0 is role=1 (mint) with nil prevTxID
		if chainEntries[0].Role != int16(models.GetTokenRoleID(constants.TokenRole_Mint)) {
			log.Fatalf("ASSERTION FAILED: %s position 0 expected role=1 (mint), got %d", tid, chainEntries[0].Role)
		}
		if chainEntries[0].PreviousTransactionID != nil {
			log.Fatalf("ASSERTION FAILED: %s position 0 expected nil prevTxID, got %v", tid, chainEntries[0].PreviousTransactionID)
		}

		// Assert all non-genesis entries have role=2 (transfer) and non-nil prevTxID
		for i := 1; i < len(chainEntries); i++ {
			if chainEntries[i].Role != int16(models.GetTokenRoleID(constants.TokenRole_Transfer)) {
				log.Fatalf("ASSERTION FAILED: %s position %d expected role=2 (transfer), got %d", tid, i, chainEntries[i].Role)
			}
			if chainEntries[i].PreviousTransactionID == nil {
				log.Fatalf("ASSERTION FAILED: %s position %d expected non-nil prevTxID, got nil", tid, i)
			}
		}

		fmt.Printf("Tokenchain assertions OK for %s\n", tid)
	}

	// Step J2: PreviousTransactionID linkage verification
	fmt.Println("\n--- Step J2: PreviousTransactionID linkage verification ---")

	for _, tid := range allTokenIDs {
		chainEntries, err := w.GetTokenChainByTokenID(tid)
		if err != nil {
			log.Fatalf("GetTokenChainByTokenID(%s) failed in step J2: %v", tid, err)
		}

		// Verify each position N+1's prevTxID == position N's txID
		for i := 1; i < len(chainEntries); i++ {
			expectedPrev := chainEntries[i-1].TransactionID
			actualPrev := chainEntries[i].PreviousTransactionID
			if actualPrev == nil || *actualPrev != expectedPrev {
				actualStr := "<nil>"
				if actualPrev != nil {
					actualStr = *actualPrev
				}
				log.Fatalf("ASSERTION FAILED: %s position %d prevTxID mismatch: expected=%s actual=%s",
					tid, i, expectedPrev, actualStr)
			}
			fmt.Printf("Chain link OK: %s position %d prevTxID == position %d txID (%s)\n",
				tid, i, i-1, expectedPrev)
		}
	}

	// Step K: tokenchain_index verification
	fmt.Println("\n--- Step K: tokenchain_index verification ---")

	expectedIndexLen := map[string]int{
		"QmTestToken001": 3, // genesis + step-F + step-H
		"QmTestToken002": 2, // genesis + step-H
		"QmTestToken003": 2, // genesis + step-H
	}

	for _, tid := range allTokenIDs {
		idx, err := w.GetTokenchainIndex(tid)
		if err != nil {
			log.Fatalf("GetTokenchainIndex(%s) failed: %v", tid, err)
		}
		if idx == nil {
			log.Fatalf("ASSERTION FAILED: tokenchain_index is nil for %s", tid)
		}
		expected := expectedIndexLen[tid]
		if len(idx.Index) != expected {
			log.Fatalf("ASSERTION FAILED: %s expected %d tokenchain_index entries, got %d", tid, expected, len(idx.Index))
		}
		fmt.Printf("tokenchain_index OK: %s has %d entries (expected %d)\n", tid, len(idx.Index), expected)
		_ = stepHTransferTxIDs[tid] // suppress unused-variable warning
	}

	fmt.Printf("\nDev runner complete: 2-DID lifecycle verified with real DIDs.\n  Sender:   %s\n  Receiver: %s\n  3 tokens minted, transferred, ownership confirmed, chains linked.\n", senderDID, receiverDID)
}
