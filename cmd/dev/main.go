package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types/models"
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
			PrivPWD: fmt.Sprintf("pwd-%d", i),
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

	for _, d := range dids {
		dc := did.InitDIDLiteWithPassword(d, cfg.DidDir, "pwd-1")

		for i := 0; i < 3; i++ {
			txInfo := &models.TransactionInfo{
				Initiator: d,
				Owner:     d,
				Epoch:     1,
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
			signatureBytes, err := dc.PvtSign(infoBytes)
			if err != nil {
				log.Fatal("PvtSign:", err)
			}
			sigStruct := &models.Signature{InitiatorSignature: hex.EncodeToString(signatureBytes)}
			sigBytes, err := json.Marshal(sigStruct)
			if err != nil {
				log.Fatal("marshal signature:", err)
			}
			txID, err := wallet.ComputeTransactionID(txInfo)
			if err != nil {
				log.Fatal("ComputeTransactionID:", err)
			}

			tx := &models.Transactions{
				ID:        txID,
				Info:      infoBytes,
				Signature: json.RawMessage(sigBytes),
			}

			token := &models.Token{
				TokenID: "", // <-- IMPORTANT: auto generate
				DID:     d,
			}

			entry := &models.TokenChain{
				Role:     1,
				Position: 0,
			}

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
