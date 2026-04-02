package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// GetTransactionsForChainSync returns serialized transactions and the next transaction ID
// for the given token starting from fromTransactionID. It wraps wallet.Wallet.GetTransactions
// for use by transaction-chain sync flows.
func GetTransactionsForChainSync(w *wallet.Wallet, tokenID, fromTransactionID string) ([][]byte, string, error) {
	transactionSyncInfo, offsetTransactionID, err := w.GetTransactionsForChainSync(tokenID, fromTransactionID)
	if err != nil {
		return nil, "", err
	}
	serializedTransactions := make([][]byte, len(transactionSyncInfo))
	for i, syncTxInfo := range transactionSyncInfo {
		serializedTransactions[i], err = json.Marshal(syncTxInfo)
		if err != nil {
			return nil, "", fmt.Errorf("GetTransactionsForChainSync: failed to marshal, err: %v", err)
		}
	}
	return serializedTransactions, offsetTransactionID, nil
}

// SyncTransactionChain handles a sync request for a token's transaction chain.
func SyncTransactionChain(req *ensweb.Request, l *ipfsport.Listener, w *wallet.Wallet, log logger.Logger) *ensweb.Result {
	var syncRequest models.TransactionChainSyncRequest
	var syncReply models.TransactionChainSyncResponse
	err := l.ParseJSON(req, &syncRequest)
	if err != nil {
		log.Error("failed to parse transaction chain sync request")
		return l.RenderJSON(req, &models.TransactionChainSyncResponse{Status: false, Message: "Failed to parse sync request"}, http.StatusOK)
	}

	syncTransactionInfo, nextTransactionID, err := GetTransactionsForChainSync(w, syncRequest.TokenID, syncRequest.TransactionID)
	if err != nil {
		log.Error("failed to get transactions")
		return l.RenderJSON(req, &models.TransactionChainSyncResponse{Status: false, Message: "Failed to get transactions"}, http.StatusOK)
	}

	syncReply.SyncTransactionInfoBytes = syncTransactionInfo
	syncReply.NextTransactionID = nextTransactionID
	syncReply.Status = true
	syncReply.Message = "Successfully got transactions"
	return l.RenderJSON(req, &syncReply, http.StatusOK)
}

func SyncTokenRecord(req *ensweb.Request, l *ipfsport.Listener, w *wallet.Wallet, log logger.Logger) *ensweb.Result {
	var syncRequest types.SyncTokenRecordRequest
	var syncReply types.SyncTokenRecordResponse
	err := l.ParseJSON(req, &syncRequest)
	if err != nil {
		log.Error("failed to parse token record sync request")
		return l.RenderJSON(req, &types.SyncTokenRecordResponse{}, http.StatusOK)
	}

	syncReply.ResultMap = make(map[string][]byte)
	for _, tokenID := range syncRequest.TokenIDs {
		tokenRecord, err := w.GetTokenByTokenID(tokenID)
		if err != nil {
			log.Error(fmt.Sprintf("failed to get token record for tokenID: %s, err: %v", tokenID, err))
			continue
		}

		tokenRecordBytes, err := json.Marshal(tokenRecord)
		if err != nil {
			log.Error(fmt.Sprintf("failed to marshal token record for tokenID: %s, err: %v", tokenID, err))
			continue
		}

		syncReply.ResultMap[tokenID] = tokenRecordBytes
	}
	return l.RenderJSON(req, &syncReply, http.StatusOK)
}
