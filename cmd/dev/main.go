package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
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

func main() {
	// ----------------------------
	// INIT CORE
	// ----------------------------
	userCfg, err := config.ParseConfigFromPath("./config")
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.CreateRubixConfigFromUserConfig(userCfg, ".")
	if err != nil {
		log.Fatal(err)
	}

	cfg.CfgData.Ports = cfg.PortConfig

	lg := logger.New(&logger.LoggerOptions{Name: "dev-v2"})

	c, err := core.NewCore(&cfg, lg, "localnet", false, false, false, "")
	if err != nil {
		log.Fatal(err)
	}

	if err := c.RunIPFS(); err != nil {
		log.Fatal(err)
	}
	defer c.StopCore()

	c.InitDIDModule()
	w := c.GetWallet()
	w.SetDidDir(cfg.DidDir)

	fmt.Println("Core + IPFS ready")

	// ----------------------------
	// CREATE 3 DIDs
	// ----------------------------
	var dids []string

	for i := 1; i <= 3; i++ {
		didID, err := c.CreateDID(&did.DIDCreate{
			PrivPWD: "pwd-1",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Created DID:", didID)
		dids = append(dids, didID)
	}

	// ----------------------------
	// MINT 5 TOKENS PER DID (D1 and D2)
	// ----------------------------
	fmt.Println("\nMinting tokens...")

	allTokenIDs := []string{}
	didTokens := make(map[string][]string)

	for _, d := range dids[:2] {
		dc := did.InitDIDLiteWithPassword(d, cfg.DidDir, "pwd-1")

		for i := 0; i < 5; i++ {
			txInfo := &models.TransactionInfo{
				Initiator: d,
				Owner:     d,
				Epoch:     int(time.Now().UnixNano()),
				Network:   "local",
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{
						{
							TokenValue: 1,
							DID:        d,
						},
					},
				},
			}

			infoBytes, err := models.SerializeTransactionInfo(txInfo)
			if err != nil {
				log.Fatal("serialize txInfo:", err)
			}
			sigHex, err := util.SignTransaction(dc, txInfo)
			if err != nil {
				log.Fatal("SignTransaction:", err)
			}
			sigStruct := &models.Signature{InitiatorSignature: sigHex}
			sigBytes, err := json.Marshal(sigStruct)
			if err != nil {
				log.Fatal("marshal signature:", err)
			}
			txID, err := wallet.ComputeTransactionID(txInfo)
			if err != nil {
				log.Fatal("ComputeTransactionID:", err)
			}
			fmt.Println("TX ID (caller):", txID)
			tx := &models.Transactions{
				ID:        txID,
				Info:      infoBytes,
				Signature: json.RawMessage(sigBytes),
			}

			token := &models.Token{
				TokenID:        "", // assigned by PersistGenesisTokenRecord
				DID:            d,
				TransactionID:  txID,
				TokenValue:     1,
				TokenStatus:    constants.TokenStatus_Free,
				TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
				LatestPosition: 0,
				LatestRole:     int16(models.GetTokenRoleID(constants.TokenRole_Mint)),
			}

			entry := &models.TokenChain{
				TransactionID: txID,
				Role:          int16(models.GetTokenRoleID(constants.TokenRole_Mint)),
				Position:      0,
			}
			fmt.Printf("DEBUG caller txID=%s  token.TransactionID=%s\n", txID, token.TransactionID)

			var sigCheck models.Signature
			if err := json.Unmarshal(tx.Signature, &sigCheck); err != nil {
				log.Fatal("unmarshal signature for verification:", err)
			}
			if err := util.VerifySignature(dc, txInfo, sigCheck.InitiatorSignature); err != nil {
				log.Fatal("pre-persistence signature verification failed:", err)
			}
			// DID binding: dc is bound to d, so d must match txInfo.Initiator and token.DID.
			if txInfo.Initiator != d {
				log.Fatal("signature: DID mismatch — signer does not match initiator")
			}
			if token.DID != txInfo.Initiator {
				log.Fatal("signature: DID mismatch — signer does not match initiator")
			}
			fmt.Println("Pre-persistence signature verified")

			err = w.PersistGenesisTokenRecord(tx, token, entry)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println("Minted:", token.TokenID, "for DID:", d)
			allTokenIDs = append(allTokenIDs, token.TokenID)
			didTokens[d] = append(didTokens[d], token.TokenID)
		}
	}

	// ----------------------------
	// VALIDATION
	// ----------------------------
	fmt.Println("\nVALIDATION START")

	validateTokenFormat(allTokenIDs)
	validateNoDuplicates(allTokenIDs)
	validateSequence(allTokenIDs)

	// ----------------------------
	// TRANSFERS
	// ----------------------------
	d1, d2, d3 := dids[0], dids[1], dids[2]

	// Round 1: Transfer all 5 D1 tokens -> D3
	fmt.Println("\n--- Round 1: D1 -> D3 ---")
	for _, tokenID := range didTokens[d1] {
		transferOneToken(w, &cfg, d1, d3, tokenID)
	}

	// Round 2: Transfer all 5 D2 tokens -> D1
	fmt.Println("\n--- Round 2: D2 -> D1 ---")
	for _, tokenID := range didTokens[d2] {
		transferOneToken(w, &cfg, d2, d1, tokenID)
	}

	// Round 3: Transfer all 5 (received from D2) D1 tokens -> D3
	// D1 now owns the 5 tokens that were originally D2's
	fmt.Println("\n--- Round 3: D1 (ex-D2 tokens) -> D3 ---")
	for _, tokenID := range didTokens[d2] {
		transferOneToken(w, &cfg, d1, d3, tokenID)
	}

	// ----------------------------
	// POST-TRANSFER VALIDATION
	// ----------------------------
	fmt.Println("\n--- Post-Transfer Validation ---")

	// All 10 tokens should now be owned by D3
	allTokensCombined := append(didTokens[d1], didTokens[d2]...)
	for _, tokenID := range allTokensCombined {
		tok, err := w.ReadToken(tokenID)
		if err != nil {
			log.Fatalf("validation: ReadToken(%s): %v", tokenID, err)
		}
		if tok.DID != d3 {
			log.Fatalf("FAIL: token %s owned by %s, expected %s", tokenID, tok.DID, d3)
		}
		fmt.Printf("  token %s: owner=%s OK\n", tokenID, tok.DID)
	}
	fmt.Println("ownership: all 10 tokens owned by D3")

	// Validate tokenchain per token
	for _, tokenID := range allTokensCombined {
		chain, err := w.GetTokenChainByTokenID(tokenID)
		if err != nil {
			log.Fatalf("validation: GetTokenChainByTokenID(%s): %v", tokenID, err)
		}
		fmt.Printf("\n  Chain for %s (%d rows):\n", tokenID, len(chain))
		for _, row := range chain {
			prevTx := "<nil>"
			if row.PreviousTransactionID != nil {
				prevTx = *row.PreviousTransactionID
			}
			fmt.Printf("    pos=%d role=%d txID=%.16s... prev=%.16s...\n",
				row.Position, row.Role, row.TransactionID, prevTx)
		}

		// Position continuity: must start at 0 and increment by 1
		for i, row := range chain {
			if row.Position != int64(i) {
				log.Fatalf("FAIL: token %s chain[%d] position=%d, expected %d",
					tokenID, i, row.Position, i)
			}
		}

		// Previous transaction linkage: chain[i].PreviousTransactionID == chain[i-1].TransactionID
		for i := 1; i < len(chain); i++ {
			if chain[i].PreviousTransactionID == nil {
				log.Fatalf("FAIL: token %s chain[%d] missing previous_transaction_id", tokenID, i)
			}
			if *chain[i].PreviousTransactionID != chain[i-1].TransactionID {
				log.Fatalf("FAIL: token %s chain[%d] prev_tx mismatch: got %s, expected %s",
					tokenID, i, *chain[i].PreviousTransactionID, chain[i-1].TransactionID)
			}
		}

		// Genesis row (pos 0) must have nil PreviousTransactionID
		if chain[0].PreviousTransactionID != nil && *chain[0].PreviousTransactionID != "" {
			log.Fatalf("FAIL: token %s genesis row has non-nil previous_transaction_id", tokenID)
		}
	}
	fmt.Println("\ntokenchain: position continuity + previous_tx linkage OK")

	// Validate token.transaction_id == latest tokenchain row transaction_id
	for _, tokenID := range allTokensCombined {
		tok, err := w.ReadToken(tokenID)
		if err != nil {
			log.Fatalf("validation: ReadToken(%s): %v", tokenID, err)
		}
		chain, err := w.GetTokenChainByTokenID(tokenID)
		if err != nil {
			log.Fatalf("validation: GetTokenChainByTokenID(%s): %v", tokenID, err)
		}
		lastChainRow := chain[len(chain)-1]
		if tok.TransactionID != lastChainRow.TransactionID {
			log.Fatalf("FAIL: token %s tx mismatch: token.TransactionID=%s, lastChain.TransactionID=%s",
				tokenID, tok.TransactionID, lastChainRow.TransactionID)
		}
	}
	fmt.Println("token-chain sync: token.transaction_id == latest chain row OK")

	// Expected chain lengths:
	// D1 tokens: genesis(0) + D1->D3(1) = 2 rows
	// D2 tokens: genesis(0) + D2->D1(1) + D1->D3(2) = 3 rows
	for _, tokenID := range didTokens[d1] {
		chain, err := w.GetTokenChainByTokenID(tokenID)
		if err != nil {
			log.Fatalf("validation: GetTokenChainByTokenID(%s): %v", tokenID, err)
		}
		if len(chain) != 2 {
			log.Fatalf("FAIL: D1 token %s expected 2 chain rows, got %d", tokenID, len(chain))
		}
	}
	for _, tokenID := range didTokens[d2] {
		chain, err := w.GetTokenChainByTokenID(tokenID)
		if err != nil {
			log.Fatalf("validation: GetTokenChainByTokenID(%s): %v", tokenID, err)
		}
		if len(chain) != 3 {
			log.Fatalf("FAIL: D2 token %s expected 3 chain rows, got %d", tokenID, len(chain))
		}
	}
	fmt.Println("chain length: D1 tokens=2 rows, D2 tokens=3 rows OK")

	fmt.Println("\nALL CHECKS PASSED - 3-DID multi-hop transfer simulation complete")
	RunStressTests(w, &cfg, dids, didTokens)
}

func validateTokenFormat(tokens []string) {
	fmt.Println("\nCheck: Token format")

	re := regexp.MustCompile(`^\d+_\d+$`)

	for _, t := range tokens {
		if !re.MatchString(t) {
			log.Fatalf("Invalid format: %s", t)
		}
	}
	fmt.Println("format OK")
}

func validateNoDuplicates(tokens []string) {
	fmt.Println("\nCheck: duplicates")

	seen := map[string]bool{}
	for _, t := range tokens {
		if seen[t] {
			log.Fatalf("duplicate token: %s", t)
		}
		seen[t] = true
	}
	fmt.Println("no duplicates")
}

func validateSequence(tokens []string) {
	fmt.Println("\nCheck: sequence continuity")

	numbers := []int{}

	for _, t := range tokens {
		parts := strings.Split(t, "_")
		num, _ := strconv.Atoi(parts[1])
		numbers = append(numbers, num)
	}

	// simple check: monotonic increase
	prev := numbers[0]
	for i := 1; i < len(numbers); i++ {
		if numbers[i] <= prev {
			log.Fatalf("non-increasing sequence: %v", numbers)
		}
		prev = numbers[i]
	}

	fmt.Println("sequence looks increasing (sanity)")
}

func transferOneToken(w *wallet.Wallet, cfg *config.Config, fromDID, toDID, tokenID string) {
	// 1. Read current token state
	tok, err := w.ReadToken(tokenID)
	if err != nil {
		log.Fatalf("transferOneToken: ReadToken(%s): %v", tokenID, err)
	}
	fmt.Printf("  Transfer %s: %s -> %s (pos=%d, txID=%s)\n",
		tokenID, fromDID, toDID, tok.LatestPosition, tok.TransactionID)

	// 2. Build TransactionInfo
	//    Initiator = fromDID (current owner, initiating transfer)
	//    Owner = toDID (new owner receiving the token)
	txInfo := &models.TransactionInfo{
		Initiator: fromDID,
		Owner:     toDID,
		Epoch:     int(time.Now().UnixNano()),
		Network:   "local",
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{
					TokenID:               tokenID,
					PreviousTransactionID: tok.TransactionID,
					TokenValue:            tok.TokenValue,
					DID:                   fromDID,
				},
			},
		},
	}

	// 3. Sign with fromDID's key
	dc := did.InitDIDLiteWithPassword(fromDID, cfg.DidDir, "pwd-1")
	sigHex, err := util.SignTransaction(dc, txInfo)
	if err != nil {
		log.Fatalf("transferOneToken: SignTransaction: %v", err)
	}

	// 4. Verify signature before persistence (pre-persistence contract)
	if err := util.VerifySignature(dc, txInfo, sigHex); err != nil {
		log.Fatalf("transferOneToken: VerifySignature: %v", err)
	}

	// 5. Build transaction record
	infoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		log.Fatalf("transferOneToken: SerializeTransactionInfo: %v", err)
	}
	sigStruct := &models.Signature{InitiatorSignature: sigHex}
	sigBytes, err := json.Marshal(sigStruct)
	if err != nil {
		log.Fatalf("transferOneToken: marshal signature: %v", err)
	}
	txID, err := wallet.ComputeTransactionID(txInfo)
	if err != nil {
		log.Fatalf("transferOneToken: ComputeTransactionID: %v", err)
	}
	tx := &models.Transactions{
		ID:        txID,
		Info:      infoBytes,
		Signature: json.RawMessage(sigBytes),
	}

	// 6. Build PostConsensusPersistenceRequest
	//    Position = tok.LatestPosition + 1 (chain continuity)
	//    PreviousTransactionID = tok.TransactionID (link to prior tx)
	newPos := tok.LatestPosition + 1
	prevTxID := tok.TransactionID

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
				Role:                  int16(models.GetTokenRoleID(constants.TokenRole_Transfer)),
				Position:              newPos,
			},
		},
		TokenStates: []models.Token{
			{
				TokenID:        tokenID,
				DID:            toDID,
				TransactionID:  txID,
				TokenValue:     tok.TokenValue,
				TokenStatus:    constants.TokenStatus_Free,
				TokenType:      tok.TokenType,
				LatestPosition: newPos,
				LatestRole:     int16(models.GetTokenRoleID(constants.TokenRole_Transfer)),
			},
		},
	}

	// 7. Persist
	if err := w.PersistPostConsensus(context.Background(), req); err != nil {
		log.Fatalf("transferOneToken: PersistPostConsensus: %v", err)
	}
	fmt.Printf("  Transferred %s -> %s (newPos=%d, txID=%s)\n", tokenID, toDID, newPos, txID)
}
