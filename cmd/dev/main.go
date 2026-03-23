package main

import (
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

	fmt.Println("✅ Core + IPFS ready")

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
	// MINT 3 TOKENS PER DID
	// ----------------------------
	fmt.Println("\n🚀 Minting tokens...")

	allTokenIDs := []string{}

	for _, d := range dids[:2] {
		dc := did.InitDIDLiteWithPassword(d, cfg.DidDir, "pwd-1")

		for i := 0; i < 3; i++ {
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
			fmt.Println("Pre-persistence signature verified")

			err = w.PersistGenesisTokenRecord(tx, token, entry)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println("Minted:", token.TokenID, "for DID:", d)
			allTokenIDs = append(allTokenIDs, token.TokenID)
		}
	}

	// ----------------------------
	// VALIDATION
	// ----------------------------
	fmt.Println("\n🔍 VALIDATION START")

	validateTokenFormat(allTokenIDs)
	validateNoDuplicates(allTokenIDs)
	validateSequence(allTokenIDs)

	fmt.Println("\n✅ ALL CHECKS PASSED")
}

func validateTokenFormat(tokens []string) {
	fmt.Println("\nCheck: Token format")

	re := regexp.MustCompile(`^\d+_\d+$`)

	for _, t := range tokens {
		if !re.MatchString(t) {
			log.Fatalf("❌ Invalid format: %s", t)
		}
	}
	fmt.Println("✔ format OK")
}

func validateNoDuplicates(tokens []string) {
	fmt.Println("\nCheck: duplicates")

	seen := map[string]bool{}
	for _, t := range tokens {
		if seen[t] {
			log.Fatalf("❌ duplicate token: %s", t)
		}
		seen[t] = true
	}
	fmt.Println("✔ no duplicates")
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
			log.Fatalf("❌ non-increasing sequence: %v", numbers)
		}
		prev = numbers[i]
	}

	fmt.Println("✔ sequence looks increasing (sanity)")
}
