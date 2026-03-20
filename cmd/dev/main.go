package main

import (
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

	fmt.Println("Dev runner complete: DID + 3 tokens seeded and verified.")
}
