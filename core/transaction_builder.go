package core

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// BuildTransactionInfoFromRequest prepares a transaction by collecting and locking tokens
// for a multi-asset transfer request. This is the single entry point for all token types:
//
//   - RBT: collected via parts.CollectRBTTokens (manages its own locking internally)
//   - FT/NFT/SC: locked in a single DB transaction — if any asset type fails,
//     all non-RBT locks roll back atomically with zero partial state
//
// Returns:
//   - *TransactionInfo with all token paths populated (Quorums and CommittedTokens are nil)
//   - float64 total token amount across all asset types (expressed in RBT)
//   - error if any path fails
func BuildTransactionInfoFromRequest(
	ctx context.Context,
	w *wallet.Wallet,
	req *models.TransactionRequest,
	dc types.DIDCrypto,
	networkMode string, // The isTestNet bool changed to networkMode string to be passed to CollectRBTTokens
	log logger.Logger,
	pubsub *types.PubSub, // punishFn which was of type func(*model.PubSubTxnInfo)  is change to types.PubSub to be passed to CollectRBTTokens
) (*models.TransactionInfo, float64, error) {

	txTokens := &models.TransactionTokens{}
	var totalAmount float64

	// --- RBT path (separate — CollectRBTTokens manages its own locking) ---
	if req.HasRBT() {
		ownerDID := dc.GetDID()
		ownedRBTTokens, err := w.LockTokensForSplit(ctx, ownerDID, req.GetRBTAmount())
		if err != nil {
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: lock RBT tokens for split: %w", err)
		}
		denomMap, err := w.GetTokenDenomArray(ownerDID)
		if err != nil {
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: get denom array: %w", err)
		}
		rbtTokens, _, _, _, err := parts.CollectRBTTokens(dc, w, req.GetRBTAmount(), ownedRBTTokens, denomMap, networkMode, log)
		if err != nil {
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: RBT collection failed: %w", err)
		}
		txTokens.RBT = rbtTokens
		totalAmount += req.GetRBTAmount()
	}

	// --- FT/NFT/SC: single DB transaction for all non-RBT assets ---
	hasNonRBT := req.HasFT() || req.HasNFT() || req.HasSmartContract()
	if hasNonRBT {
		tx, err := w.BeginTx(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: begin tx: %w", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: set lock_timeout: %w", err)
		}

		// Collect all locked token IDs for the final batch UPDATE
		var allLockedIDs []string

		// FT path
		if req.HasFT() {
			for _, ftInfo := range req.GetAllFTs() {
				selected, err := w.QueryAndLockFTs(ctx, tx, req.Initiator, ftInfo.FTName, ftInfo.CreatorDID, int(ftInfo.NumberOfFts))
				if err != nil {
					return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: FT lock failed for ft_name=%s: %w", ftInfo.FTName, err)
				}
				for _, tok := range selected {
					txTokens.FT = append(txTokens.FT, &models.TokenInfo{
						TokenID:               tok.TokenID,
						PreviousTransactionID: tok.TransactionID,
					})
					allLockedIDs = append(allLockedIDs, tok.TokenID)
					totalAmount += tok.TokenValue
				}
			}
		}

		// NFT path
		if req.HasNFT() {
			allNFTs := req.GetAllNFTs()
			nftIDs := make([]string, len(allNFTs))
			nftDataMap := make(map[string]string, len(allNFTs))
			for i, nftInfo := range allNFTs {
				nftIDs[i] = nftInfo.NFTId
				nftDataMap[nftInfo.NFTId] = nftInfo.Data
			}
			sort.Strings(nftIDs)

			locked, err := w.QueryAndLockByIDs(ctx, tx, req.Initiator, nftIDs, constants.TokenType_NFT)
			if err != nil {
				return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: NFT lock failed: %w", err)
			}
			for _, tok := range locked {
				txTokens.NFT = append(txTokens.NFT, &models.TokenInfo{
					TokenID:               tok.TokenID,
					PreviousTransactionID: tok.TransactionID,
					Data:                  nftDataMap[tok.TokenID],
				})
				allLockedIDs = append(allLockedIDs, tok.TokenID)
				totalAmount += tok.TokenValue
			}
		}

		// SmartContract path
		if req.HasSmartContract() {
			allSCs := req.GetAllSmartContracts()
			scIDs := make([]string, len(allSCs))
			scDataMap := make(map[string]string, len(allSCs))
			for i, scInfo := range allSCs {
				scIDs[i] = scInfo.SmartContractId
				scDataMap[scInfo.SmartContractId] = scInfo.Data
			}
			sort.Strings(scIDs)

			locked, err := w.QueryAndLockByIDs(ctx, tx, req.Initiator, scIDs, constants.TokenType_SmartContract)
			if err != nil {
				return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: SC lock failed: %w", err)
			}
			for _, tok := range locked {
				txTokens.SmartContract = append(txTokens.SmartContract, &models.TokenInfo{
					TokenID:               tok.TokenID,
					PreviousTransactionID: tok.TransactionID,
					Data:                  scDataMap[tok.TokenID],
				})
				allLockedIDs = append(allLockedIDs, tok.TokenID)
				totalAmount += tok.TokenValue
			}
		}

		// Single batch UPDATE for all locked tokens, then COMMIT
		if len(allLockedIDs) > 0 {
			_, err = tx.Exec(ctx,
				`UPDATE tokens SET token_status = $1, updated_at = $2 WHERE token_id = ANY($3::text[])`,
				constants.TokenStatus_Locked, time.Now(), allLockedIDs,
			)
			if err != nil {
				return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: update status: %w", err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: commit: %w", err)
		}
	}

	txInfo := &models.TransactionInfo{
		Initiator:       req.Initiator,
		Owner:           req.Owner,
		Epoch:           int(time.Now().Unix()),
		Tokens:          txTokens,
		CommittedTokens: nil,
		Quorums:         nil,
		Memo:            req.Memo,
	}

	return txInfo, totalAmount, nil
}
