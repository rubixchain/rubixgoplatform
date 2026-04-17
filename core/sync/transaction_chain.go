package sync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const APISyncTransactionChain = "/api/sync-transaction-chain"

// GetTransactionsForChainSync returns serialized transactions and the next transaction ID
// for the given token starting from fromTransactionID. It wraps wallet.Wallet.GetTransactions
// for use by transaction-chain sync flows.
func GetTransactionsForChainSync(w *wallet.Wallet, tokenID, fromTransactionID string) ([][]byte, string, error) {
	return w.GetTransactions(tokenID, fromTransactionID)
}

// SyncTransactionChain handles a sync request for a token's transaction chain.
func SyncTransactionChain(req *ensweb.Request, l *ipfsport.Listener, w *wallet.Wallet, log logger.Logger) *ensweb.Result {
	var syncRequest models.TransactionChainSyncRequest
	var syncReply models.TransactionChainSyncReply
	err := l.ParseJSON(req, &syncRequest)
	if err != nil {
		log.Error("failed to parse transaction chain sync request")
		return l.RenderJSON(req, &models.TransactionChainSyncReply{Status: false, Message: "Failed to parse sync request"}, http.StatusOK)
	}
	transactions, nextTransactionID, err := GetTransactionsForChainSync(w, syncRequest.TokenID, syncRequest.TransactionID)
	if err != nil {
		log.Error("failed to get transactions")
		return l.RenderJSON(req, &models.TransactionChainSyncReply{Status: false, Message: "Failed to get transactions"}, http.StatusOK)
	}
	syncReply.Transactions = transactions
	syncReply.NextTransactionID = nextTransactionID
	syncReply.Status = true
	syncReply.Message = "Successfully got transactions"
	return l.RenderJSON(req, &syncReply, http.StatusOK)
}

// FindTokenRoleInTxn determines the role a token played in a given transaction.
func FindTokenRoleInTxn(tokenID string, txInfo *models.TransactionInfo) int16 {
	if txInfo.Tokens != nil {
		for _, lists := range [][]*models.TokenInfo{
			txInfo.Tokens.RBT, txInfo.Tokens.NFT,
			txInfo.Tokens.FT, txInfo.Tokens.SmartContract,
		} {
			for _, t := range lists {
				if t.TokenID == tokenID {
					return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
				}
			}
		}
	}

	for _, t := range txInfo.CommittedTokens {
		if t.TokenID == tokenID {
			return int16(models.GetTokenRoleID(constants.TokenRole_Commit))
		}
	}

	for _, q := range txInfo.Quorums {
		for _, t := range q.Tokens {
			if t.TokenID == tokenID {
				return int16(models.GetTokenRoleID(constants.TokenRole_Pledge))
			}
		}
	}

	return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
}

// SyncTransactionChainFrom fetches missing transactions from a peer and writes them locally.
func SyncTransactionChainFrom(p *ipfsport.Peer, tokenID string, tokenType int, w *wallet.Wallet, log logger.Logger) (error, *models.TransactionChainSyncReply) {
	var err error

	latestTransactionID := w.GetLatestTransactionID(tokenID,false)
	if latestTransactionID == "" {
		log.Error("failed to get latest transaction id")
		return err, nil
	}

	syncReq := models.TransactionChainSyncRequest{
		TokenID:       tokenID,
		TransactionID: latestTransactionID,
	}

	for {
		var trep models.TransactionChainSyncReply
		err = p.SendJSONRequest("POST", APISyncTransactionChain, nil, &syncReq, &trep, false)
		if err != nil {
			log.Error("failed to sync transaction chain")
			return err, nil
		}
		if !trep.Status {
			log.Error("failed to sync transaction chain")
			return fmt.Errorf(trep.Message), nil
		}
		if len(trep.Transactions) > 0 {
			for _, txn := range trep.Transactions {
				tx, err := util.TransactionFromBytes(txn)
				if tx == nil {
					log.Error("failed to convert transaction bytes to transaction")
					return fmt.Errorf("failed to convert transaction bytes to transaction"), nil
				}
				var txInfo models.TransactionInfo
				if err = json.Unmarshal(tx.Info, &txInfo); err != nil {
					log.Error("failed to unmarshal transaction info", "err", err)
					return fmt.Errorf("failed to unmarshal transaction info: %w", err), nil
				}

				role := FindTokenRoleInTxn(tokenID, &txInfo)

				if err = w.CreateTransaction(tx); err != nil {
					log.Error("failed to add transaction to transactions table", "err", err)
					return fmt.Errorf("failed to add transaction: %w", err), nil
				}

				tokenDetails, err := w.GetTokenByTokenID(tokenID)
				if err != nil {
					newToken := models.Token{
						TokenID:        tokenID,
						TokenStatus:    constants.TokenStatus_Free,
						DID:            txInfo.Owner,
						TransactionID:  tx.ID,
						TokenType:      int16(tokenType),
						LatestPosition: 0,
						LatestRole:     role,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}
					if createErr := w.CreateToken(&newToken); createErr != nil {
						log.Error("failed to create token", "err", createErr)
						return fmt.Errorf("failed to create token: %w", createErr), nil
					}
					tokenDetails = newToken
				} else {
					tokenDetails.DID = txInfo.Owner
					tokenDetails.TransactionID = tx.ID
					tokenDetails.LatestPosition++
					tokenDetails.LatestRole = role
					if updateErr := w.UpdateToken(tokenDetails); updateErr != nil {
						log.Error("failed to update token", "err", updateErr)
						return fmt.Errorf("failed to update token: %w", updateErr), nil
					}
				}

				entry := &models.TokenChain{
					TokenID:       tokenID,
					TransactionID: tx.ID,
					Role:          role,
					Position:      tokenDetails.LatestPosition,
				}
				if err = w.AddTokenChainEntry(entry); err != nil {
					log.Error("failed to add token chain entry", "err", err)
					return fmt.Errorf("failed to add token chain entry: %w", err), nil
				}
			}
		}
		if trep.NextTransactionID == "" {
			break
		}
		syncReq.TransactionID = trep.NextTransactionID
	}
	return nil, nil
}
