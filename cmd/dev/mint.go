package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func mintTokens(w *wallet.Wallet, cfg *types.RubixConfig, dids []string, count int) ([]string, map[string][]string) {
	fmt.Println("\nMinting tokens...")

	allTokenIDs := []string{}
	didTokens := make(map[string][]string)

	for _, d := range dids {
		dc := did.InitDIDLiteWithPassword(d, cfg.DidDir, "pwd-1")

		for range count {
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
			txID, err := util.GetTransactionID(txInfo)
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
			if txInfo.Initiator != d {
				log.Fatal("signature: DID mismatch — signer does not match initiator")
			}
			if token.DID != txInfo.Initiator {
				log.Fatal("signature: DID mismatch — signer does not match initiator")
			}
			fmt.Println("Pre-persistence signature verified")

			if err := w.PersistGenesisTokenRecord(tx, token, entry); err != nil {
				log.Fatal(err)
			}

			fmt.Println("Minted:", token.TokenID, "for DID:", d)
			allTokenIDs = append(allTokenIDs, token.TokenID)
			didTokens[d] = append(didTokens[d], token.TokenID)
		}
	}

	return allTokenIDs, didTokens
}
