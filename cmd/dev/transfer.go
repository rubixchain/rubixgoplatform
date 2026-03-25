package main

import (
	"context"
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

func runTransferRounds(w *wallet.Wallet, cfg *types.RubixConfig, dids []string, didTokens map[string][]string) {
	d1, d2, d3 := dids[0], dids[1], dids[2]

	// Round 1: Transfer all 5 D1 tokens -> D3
	fmt.Println("\n--- Round 1: D1 -> D3 ---")
	for _, tokenID := range didTokens[d1] {
		transferOneToken(w, cfg, d1, d3, tokenID)
	}

	// Round 2: Transfer all 5 D2 tokens -> D1
	fmt.Println("\n--- Round 2: D2 -> D1 ---")
	for _, tokenID := range didTokens[d2] {
		transferOneToken(w, cfg, d2, d1, tokenID)
	}

	// Round 3: Transfer all 5 (received from D2) D1 tokens -> D3
	// D1 now owns the 5 tokens that were originally D2's
	fmt.Println("\n--- Round 3: D1 (ex-D2 tokens) -> D3 ---")
	for _, tokenID := range didTokens[d2] {
		transferOneToken(w, cfg, d1, d3, tokenID)
	}
}

func transferOneToken(w *wallet.Wallet, cfg *types.RubixConfig, fromDID, toDID, tokenID string) {
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
	txID, err := util.GetTransactionID(txInfo)
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
