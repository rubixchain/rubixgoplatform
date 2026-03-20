package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func main() {
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

	// Step 3: Create logger
	lg := logger.New(&logger.LoggerOptions{Name: "dev"})

	// Step 4: Call NewCore -- note &cfg (address-of value)
	c, err := core.NewCore(&cfg, lg, "localnet", false, false, false, "")
	if err != nil {
		log.Fatalf("failed to create core: %v", err)
	}

	fmt.Println("Core initialized successfully.")

	// Step A: Get wallet reference
	w := c.GetWallet()

	// Step B: Insert test DID (idempotent via ON CONFLICT UPDATE)
	didInfo := &models.DID{
		DID:    "bafybmidtest1234",
		PeerID: "",
		Local:  true,
		AlgoID: int64(models.GetDidAlgoType(constants.DidAlgo_SECP256K1)),
	}
	if err := w.CreateOrUpdateDID(didInfo); err != nil {
		log.Fatalf("CreateOrUpdateDID failed: %v", err)
	}
	fmt.Println("DID inserted/updated: bafybmidtest1234")

	// Step C: Seed 3 genesis RBT tokens (idempotent -- check existence first)
	for i := 1; i <= 3; i++ {
		tokenID := fmt.Sprintf("QmTestToken%03d", i)

		existing, readErr := w.ReadToken(tokenID)
		if readErr == nil && existing != nil {
			fmt.Printf("Token %s already exists, skipping.\n", tokenID)
			continue
		}

		// Token not found -- build and persist
		txInfo := &models.TransactionInfo{
			Initiator: "bafybmidtest1234",
			Owner:     "bafybmidtest1234",
			Epoch:     0,
			Network:   constants.NetworkID_RBT_Local,
			Tokens: &models.TransactionTokens{
				RBT: []*models.TokenInfo{
					{
						TokenID:    tokenID,
						TokenValue: 1.0,
						DID:        "bafybmidtest1234",
					},
				},
			},
		}

		txID, err := wallet.ComputeTransactionID(txInfo)
		if err != nil {
			log.Fatalf("ComputeTransactionID failed for %s: %v", tokenID, err)
		}

		infoBytes, err := models.SerializeTransactionInfo(txInfo)
		if err != nil {
			log.Fatalf("SerializeTransactionInfo failed for %s: %v", tokenID, err)
		}

		txRecord := &models.Transactions{
			ID:        txID,
			Info:      infoBytes,
			Signature: []byte("{}"),
		}

		token := &models.Token{
			TokenID:        tokenID,
			ParentTokenID:  pgtype.Text{},
			TokenValue:     1.0,
			TokenStatus:    int16(constants.TokenStatus_Free),
			DID:            "bafybmidtest1234",
			TransactionID:  txID,
			TokenStateHash: "",
			TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
			LatestPosition: 0,
			LatestRole:     int16(models.GetTokenRoleID(constants.TokenRole_Mint)),
		}

		entry := &models.TokenChain{
			TokenID:               tokenID,
			TransactionID:         txID,
			PreviousTransactionID: nil,
			Role:                  int16(models.GetTokenRoleID(constants.TokenRole_Mint)),
			Position:              0,
		}

		if err := w.PersistGenesisTokenRecord(txRecord, token, entry); err != nil {
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

	// Step D: Verify by reading back free tokens for the test DID
	tokens, tokenIDs, err := w.GetFreeRBTTokens("bafybmidtest1234")
	if err != nil {
		log.Fatalf("GetFreeRBTTokens failed: %v", err)
	}
	fmt.Printf("Free RBT tokens for bafybmidtest1234: count=%d, ids=%v\n", len(tokens), tokenIDs)

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
		Initiator: "bafybmidtest1234",
		Owner:     "bafybmidtest1234", // same DID for simplicity (initiator perspective)
		Epoch:     1,
		Network:   constants.NetworkID_RBT_Local,
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{
					TokenID:               "QmTestToken001",
					PreviousTransactionID: preToken.TransactionID,
					TokenValue:            1.0,
					DID:                   "bafybmidtest1234",
				},
			},
		},
	}

	// F3: Compute transaction ID for logging
	transferTxID, err := wallet.ComputeTransactionID(transferTxInfo)
	if err != nil {
		log.Fatalf("ComputeTransactionID for transfer failed: %v", err)
	}
	fmt.Printf("Transfer transaction ID: %s\n", transferTxID)

	// F4: Build signature and call PersistPostConsensus (production 4-field pattern)
	transferSig := &models.Signature{
		InitiatorSignature: "dev-test-transfer-sig",
		Quorums:            nil,
	}

	ctx := context.Background()
	if err := w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
		TransactionInfo: transferTxInfo,
		Signature:       transferSig,
		DID:             "bafybmidtest1234",
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

	fmt.Println("\nDev runner complete: DID + 3 tokens seeded + transfer simulated and verified.")
}
