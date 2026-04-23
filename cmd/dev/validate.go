package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func validateMintedTokens(allTokenIDs []string) {
	validateTokenFormat(allTokenIDs)
	validateNoDuplicates(allTokenIDs)
	validateSequence(allTokenIDs)
}

func validatePostTransfer(w *wallet.Wallet, dids []string, didTokens map[string][]string) {
	d1, d2, d3 := dids[0], dids[1], dids[2]

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
		chain, err := w.GetTokenChainByTokenID(tokenID, false)
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
		chain, err := w.GetTokenChainByTokenID(tokenID, false)
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
		chain, err := w.GetTokenChainByTokenID(tokenID, false)
		if err != nil {
			log.Fatalf("validation: GetTokenChainByTokenID(%s): %v", tokenID, err)
		}
		if len(chain) != 2 {
			log.Fatalf("FAIL: D1 token %s expected 2 chain rows, got %d", tokenID, len(chain))
		}
	}
	for _, tokenID := range didTokens[d2] {
		chain, err := w.GetTokenChainByTokenID(tokenID, false)
		if err != nil {
			log.Fatalf("validation: GetTokenChainByTokenID(%s): %v", tokenID, err)
		}
		if len(chain) != 3 {
			log.Fatalf("FAIL: D2 token %s expected 3 chain rows, got %d", tokenID, len(chain))
		}
	}
	fmt.Println("chain length: D1 tokens=2 rows, D2 tokens=3 rows OK")
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
	prev := numbers[0]
	for i := 1; i < len(numbers); i++ {
		if numbers[i] <= prev {
			log.Fatalf("non-increasing sequence: %v", numbers)
		}
		prev = numbers[i]
	}
	fmt.Println("sequence looks increasing (sanity)")
}
