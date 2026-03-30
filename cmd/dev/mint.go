package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
)

func mintTokens(w *wallet.Wallet, cfg *types.RubixConfig, dids []string, count int) ([]string, map[string][]string) {
	fmt.Println("\nMinting tokens...")

	allTokenIDs := []string{}
	didTokens := make(map[string][]string)

	ctx := context.Background()
	for _, d := range dids {
		dc := did.InitDIDLiteWithPassword(d, cfg.DidDir, "pwd-1")

		for i := 1; i <= count; i++ {
			currentTime := int(time.Now().Unix())

			tx, err := w.BeginTx(ctx)
			if err != nil {
				fmt.Printf("PersistGenesisTokenRecord: begin tx: %w", err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck

			tokenID := fmt.Sprintf("QmTestToken%03d", i)

			if _, err := w.PersistGenesisTokenRecord(tx, dc, nil, tokenID, d, constants.NetworkMode_Localnet, currentTime); err != nil {
				log.Fatal(err)
			}

			fmt.Println("Minted:", tokenID, "for DID:", d)
			allTokenIDs = append(allTokenIDs, tokenID)
			didTokens[d] = append(didTokens[d], tokenID)
		}
	}

	return allTokenIDs, didTokens
}
