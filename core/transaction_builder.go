package core

import (
	"context"
	"fmt"
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
	var allCommittedTokens []*models.TokenInfo

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

		// NFT path - handle both deployment and execution
		if req.HasNFT() {
			allNFTs := req.GetAllNFTs()

			for _, nftInfo := range allNFTs {
				// Check if NFT token exists in database
				exists, err := w.TokenExists(ctx, tx, nftInfo.NFTId)
				if err != nil {
					return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: failed to check NFT existence: %w", err)
				}

				if exists {
					// EXECUTION MODE: Token exists, lock it for execution
					locked, err := w.QueryAndLockByIDs(ctx, tx, req.Initiator, []string{nftInfo.NFTId}, constants.TokenType_NFT)
					if err != nil {
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: NFT lock failed for %s: %w", nftInfo.NFTId, err)
					}
					if len(locked) == 0 {
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: NFT %s exists but could not be locked", nftInfo.NFTId)
					}
					tok := locked[0]
					txTokens.NFT = append(txTokens.NFT, &models.TokenInfo{
						TokenID:               tok.TokenID,
						PreviousTransactionID: tok.TransactionID,
						Data:                  nftInfo.Data,
					})
					allLockedIDs = append(allLockedIDs, tok.TokenID)
					totalAmount += tok.TokenValue
				} else {
					// DEPLOYMENT MODE: Token doesn't exist, prepare for deployment
					// Add NFT to transaction tokens (will be deployed during consensus)
					txTokens.NFT = append(txTokens.NFT, &models.TokenInfo{
						TokenID:               nftInfo.NFTId,
						PreviousTransactionID: "", // Genesis - no previous transaction
						Data:                  nftInfo.Data,
					})
					totalAmount += nftInfo.Value
				}
			}
		}

		// SmartContract path - handle both deployment and execution
		if req.HasSmartContract() {
			allSCs := req.GetAllSmartContracts()

			for _, scInfo := range allSCs {
				// Check if SC token exists in database
				exists, err := w.TokenExists(ctx, tx, scInfo.SmartContractId)
				if err != nil {
					return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: failed to check SC existence: %w", err)
				}

				if exists {
					// EXECUTION MODE: Token exists, lock it for execution
					locked, err := w.QueryAndLockByIDs(ctx, tx, req.Initiator, []string{scInfo.SmartContractId}, constants.TokenType_SmartContract)
					if err != nil {
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: SC lock failed for %s: %w", scInfo.SmartContractId, err)
					}
					if len(locked) == 0 {
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: SC %s exists but could not be locked", scInfo.SmartContractId)
					}
					tok := locked[0]
					txTokens.SmartContract = append(txTokens.SmartContract, &models.TokenInfo{
						TokenID:               tok.TokenID,
						PreviousTransactionID: tok.TransactionID,
						Data:                  scInfo.Data,
					})
					allLockedIDs = append(allLockedIDs, tok.TokenID)
					totalAmount += tok.TokenValue
				} else {
					// DEPLOYMENT MODE: Token doesn't exist, prepare for deployment
					// Collect committed tokens if SC has value > 0
					if scInfo.Value > 0 {
						// Lock RBT tokens for the smart contract value
						ownedRBTTokens, err := w.LockTokensForSplit(ctx, req.Initiator, scInfo.Value)
						if err != nil {
							return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: failed to lock RBT for SC committed tokens: %w", err)
						}

						// Convert to TokenInfo format for CommittedTokens and add to locked list
						for _, token := range ownedRBTTokens {
							committedToken := &models.TokenInfo{
								TokenID:               token.TokenID,
								PreviousTransactionID: token.TransactionID,
							}
							allCommittedTokens = append(allCommittedTokens, committedToken)
							// Add to locked list so they get marked as Locked (not Committed yet - that happens during consensus)
							allLockedIDs = append(allLockedIDs, token.TokenID)
						}
					}

					// Add SC to transaction tokens (will be deployed during consensus)
					txTokens.SmartContract = append(txTokens.SmartContract, &models.TokenInfo{
						TokenID:               scInfo.SmartContractId,
						PreviousTransactionID: "", // Genesis - no previous transaction
						Data:                  scInfo.Data,
					})
					totalAmount += scInfo.Value
				}
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

	// Set CommittedTokens from SC deployments (will be nil if no SC deployments occurred)
	var committedTokens []*models.TokenInfo
	if len(allCommittedTokens) > 0 {
		committedTokens = allCommittedTokens
	}

	txInfo := &models.TransactionInfo{
		Network:         networkMode,
		Initiator:       req.Initiator,
		Owner:           req.Owner,
		Epoch:           int(time.Now().Unix()),
		Tokens:          txTokens,
		CommittedTokens: committedTokens,
		Quorums:         nil,
		Memo:            req.Memo,
	}

	return txInfo, totalAmount, nil
}
