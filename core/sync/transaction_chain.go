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

const APISyncTransactionChain = "/rubix/v1/internal/sync_transaction_chain"

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
// transferNFTOwnership controls NFT role: false=Execute, true=Transfer.
// This replaces the old Owner==Initiator heuristic which was incorrect in
// mixed transactions where Owner is the RBT receiver, not the NFT owner.
func FindTokenRoleInTxn(tokenID string, txInfo *models.TransactionInfo, transferNFTOwnership bool) int16 {
	if txInfo.Tokens != nil {
		for _, t := range txInfo.Tokens.RBT {
			if t.TokenID == tokenID {
				return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
			}
		}
		for _, t := range txInfo.Tokens.NFT {
			if t.TokenID == tokenID {
				if t.PreviousTransactionID == "" {
					return int16(models.GetTokenRoleID(constants.TokenRole_Deploy))
				}
				if !transferNFTOwnership {
					return int16(models.GetTokenRoleID(constants.TokenRole_Execute))
				}
				return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
			}
		}
		for _, t := range txInfo.Tokens.FT {
			if t.TokenID == tokenID {
				return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
			}
		}
		for _, t := range txInfo.Tokens.SmartContract {
			if t.TokenID == tokenID {
				// Genesis (empty PreviousTransactionID) = Deploy; otherwise Execute.
				if t.PreviousTransactionID == "" {
					return int16(models.GetTokenRoleID(constants.TokenRole_Deploy))
				}
				return int16(models.GetTokenRoleID(constants.TokenRole_Execute))
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

// findTokenValue returns the TokenValue for tokenID from any token list in txInfo.
// Returns 0 if not found.
func findTokenValue(tokenID string, txInfo *models.TransactionInfo) float64 {
	if txInfo.Tokens != nil {
		for _, t := range txInfo.Tokens.RBT {
			if t.TokenID == tokenID {
				return t.TokenValue
			}
		}
		for _, t := range txInfo.Tokens.NFT {
			if t.TokenID == tokenID {
				return t.TokenValue
			}
		}
		for _, t := range txInfo.Tokens.FT {
			if t.TokenID == tokenID {
				return t.TokenValue
			}
		}
		for _, t := range txInfo.Tokens.SmartContract {
			if t.TokenID == tokenID {
				return t.TokenValue
			}
		}
	}
	return 0
}

// findPreviousTransactionID returns the PreviousTransactionID for tokenID from
// any token list in txInfo. Returns "" if not found (genesis case).
func findPreviousTransactionID(tokenID string, txInfo *models.TransactionInfo) string {
	if txInfo.Tokens != nil {
		for _, lists := range [][]*models.TokenInfo{
			txInfo.Tokens.RBT, txInfo.Tokens.NFT,
			txInfo.Tokens.FT, txInfo.Tokens.SmartContract,
		} {
			for _, t := range lists {
				if t.TokenID == tokenID {
					return t.PreviousTransactionID
				}
			}
		}
	}
	return ""
}

// SyncTransactionChainFrom fetches missing transactions from a peer and writes them locally.
// If the token is not yet known locally, latestTransactionID will be "" which signals
// the server to return the full chain from genesis.
func SyncTransactionChainFrom(p *ipfsport.Peer, tokenID string, tokenType int, w *wallet.Wallet, log logger.Logger) (error, *models.TransactionChainSyncReply) {
	var err error

	// "" means token not in local DB yet — server interprets "" as "send from genesis".
	latestTransactionID := w.GetLatestTransactionID(tokenID, false)

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

				role := FindTokenRoleInTxn(tokenID, &txInfo, false)

				// Derive the correct token status from the role, mirroring
				// post_consensus_payload_builder.go so the synced record matches
				// what the originating node wrote.
				tokenStatus := int16(constants.TokenStatus_Free)
				switch role {
				case int16(models.GetTokenRoleID(constants.TokenRole_Deploy)):
					tokenStatus = int16(constants.TokenStatus_Deployed)
				case int16(models.GetTokenRoleID(constants.TokenRole_Execute)):
					tokenStatus = int16(constants.TokenStatus_Executed)
				// A consumed parent must not default to Free, or it can be re-selected
				// by LockTokensForSplit and re-split into a duplicate genesis
				// (double-mint). Mirror the originating node's status:
				// Commit -> Committed, Burn -> Burnt. See docs/PROBLEM.md.
				case int16(models.GetTokenRoleID(constants.TokenRole_Commit)):
					tokenStatus = int16(constants.TokenStatus_Committed)
				case int16(models.GetTokenRoleID(constants.TokenRole_Burn)):
					tokenStatus = int16(constants.TokenStatus_Burnt)
				}

				if err = w.CreateTransaction(tx); err != nil {
					log.Error("failed to add transaction to transactions table", "err", err)
					return fmt.Errorf("failed to add transaction: %w", err), nil
				}

				// Ensure the owner DID exists in the dids table before writing the
				// token — tokens.did has a FK to dids.did. The owner may be a remote
				// peer whose DID was never registered locally, so we upsert it as a
				// non-local entry. We must also resolve algo_id (FK to did_algo) since
				// the zero value violates the algo_id_fk constraint.
				ownerDID := txInfo.Owner
				if ownerDID == "" {
					ownerDID = txInfo.Initiator
				}
				if ownerDID != "" {
					algoID, algoErr := w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
					if algoErr != nil {
						log.Error("failed to resolve algo ID for owner DID upsert", "did", ownerDID, "err", algoErr)
						return fmt.Errorf("failed to resolve algo ID for DID %s: %w", ownerDID, algoErr), nil
					}
					if didErr := w.CreateOrUpdateDID(&models.DID{
						DID:    ownerDID,
						Local:  false,
						AlgoID: algoID,
					}); didErr != nil {
						log.Error("failed to upsert owner DID before creating token", "did", ownerDID, "err", didErr)
						return fmt.Errorf("failed to upsert owner DID %s: %w", ownerDID, didErr), nil
					}
				}

				tokenValue := findTokenValue(tokenID, &txInfo)

				tokenDetails, err := w.GetTokenByTokenID(tokenID)
				if err != nil {
					newToken := models.Token{
						TokenID:        tokenID,
						TokenStatus:    tokenStatus,
						TokenValue:     tokenValue,
						DID:            ownerDID,
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
					tokenDetails.DID = ownerDID
					tokenDetails.TokenStatus = tokenStatus
					tokenDetails.TokenValue = tokenValue
					tokenDetails.TransactionID = tx.ID
					tokenDetails.LatestPosition++
					tokenDetails.LatestRole = role
					if updateErr := w.UpdateToken(tokenDetails); updateErr != nil {
						log.Error("failed to update token", "err", updateErr)
						return fmt.Errorf("failed to update token: %w", updateErr), nil
					}
				}

				// Populate PreviousTransactionID from the token's own chain history.
				// position=0 (genesis) must be NULL; position>0 must be a valid tx ID.
				var prevTxID *string
				if raw := findPreviousTransactionID(tokenID, &txInfo); raw != "" {
					prevTxID = &raw
				}
				entry := &models.TokenChain{
					TokenID:               tokenID,
					TransactionID:         tx.ID,
					PreviousTransactionID: prevTxID,
					Role:                  role,
					Position:              tokenDetails.LatestPosition,
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
