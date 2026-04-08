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
	log.Info("BuildTransactionInfoFromRequest: Starting",
		"hasRBT", req.HasRBT(),
		"hasFT", req.HasFT(),
		"hasNFT", req.HasNFT(),
		"hasSC", req.HasSmartContract(),
		"rbtAmount", req.GetRBTAmount(),
		"initiator", req.Initiator,
		"owner", req.Owner,
	)

	txTokens := &models.TransactionTokens{}
	var totalAmount float64
	var allCommittedTokens []*models.TokenInfo

	// --- RBT path (separate — CollectRBTTokens manages its own locking) ---
	if req.HasRBT() {
		log.Info("BuildTransactionInfoFromRequest: Processing RBT tokens", "amount", req.GetRBTAmount())
		ownerDID := dc.GetDID()
		log.Debug("BuildTransactionInfoFromRequest: Locking RBT tokens for split", "did", ownerDID, "amount", req.GetRBTAmount())
		ownedRBTTokens, err := w.LockTokensForSplit(ctx, ownerDID, req.GetRBTAmount())
		if err != nil {
			log.Error("BuildTransactionInfoFromRequest: Failed to lock RBT tokens", "err", err, "did", ownerDID, "amount", req.GetRBTAmount())
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: lock RBT tokens for split: %w", err)
		}
		log.Info("BuildTransactionInfoFromRequest: RBT tokens locked", "count", len(ownedRBTTokens), "did", ownerDID)

		log.Debug("BuildTransactionInfoFromRequest: Getting token denom array", "did", ownerDID)
		denomMap, err := w.GetTokenDenomArray(ownerDID)
		if err != nil {
			log.Error("BuildTransactionInfoFromRequest: Failed to get denom array", "err", err, "did", ownerDID)
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: get denom array: %w", err)
		}
		log.Debug("BuildTransactionInfoFromRequest: Denom map retrieved", "denomCount", len(denomMap))

		log.Debug("BuildTransactionInfoFromRequest: Collecting RBT tokens", "amount", req.GetRBTAmount(), "lockedTokens", len(ownedRBTTokens))
		rbtTokens, _, _, _, err := parts.CollectRBTTokens(dc, w, req.GetRBTAmount(), ownedRBTTokens, denomMap, networkMode, log)
		if err != nil {
			log.Error("BuildTransactionInfoFromRequest: RBT collection failed", "err", err, "amount", req.GetRBTAmount())
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: RBT collection failed: %w", err)
		}
		txTokens.RBT = rbtTokens
		totalAmount += req.GetRBTAmount()
		log.Info("BuildTransactionInfoFromRequest: RBT tokens collected", "tokenCount", len(rbtTokens), "amount", req.GetRBTAmount())
	}

	// --- FT/NFT/SC: single DB transaction for all non-RBT assets ---
	hasNonRBT := req.HasFT() || req.HasNFT() || req.HasSmartContract()
	if hasNonRBT {
		log.Info("BuildTransactionInfoFromRequest: Processing non-RBT assets", "hasFT", req.HasFT(), "hasNFT", req.HasNFT(), "hasSC", req.HasSmartContract())
		tx, err := w.BeginTx(ctx)
		if err != nil {
			log.Error("BuildTransactionInfoFromRequest: Failed to begin transaction", "err", err)
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: begin tx: %w", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
			log.Error("BuildTransactionInfoFromRequest: Failed to set lock timeout", "err", err)
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: set lock_timeout: %w", err)
		}
		log.Debug("BuildTransactionInfoFromRequest: DB transaction started with lock_timeout=5s")

		// Collect all locked token IDs for the final batch UPDATE
		var allLockedIDs []string

		// FT path
		if req.HasFT() {
			log.Info("BuildTransactionInfoFromRequest: Processing FT tokens", "ftCount", len(req.GetAllFTs()))
			for _, ftInfo := range req.GetAllFTs() {
				log.Debug("BuildTransactionInfoFromRequest: Locking FT tokens", "ftName", ftInfo.FTName, "creator", ftInfo.CreatorDID, "count", ftInfo.NumberOfFts)
				selected, err := w.QueryAndLockFTs(ctx, tx, req.Initiator, ftInfo.FTName, ftInfo.CreatorDID, int(ftInfo.NumberOfFts))
				if err != nil {
					log.Error("BuildTransactionInfoFromRequest: FT lock failed", "err", err, "ftName", ftInfo.FTName, "creator", ftInfo.CreatorDID)
					return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: FT lock failed for ft_name=%s: %w", ftInfo.FTName, err)
				}
				log.Info("BuildTransactionInfoFromRequest: FT tokens locked", "ftName", ftInfo.FTName, "count", len(selected))
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
			log.Info("BuildTransactionInfoFromRequest: Processing NFT tokens", "nftCount", len(allNFTs))

			for _, nftInfo := range allNFTs {
				log.Debug("BuildTransactionInfoFromRequest: Checking NFT existence", "nftID", nftInfo.NFTId)
				// Check if NFT token exists in database
				exists, err := w.TokenExists(ctx, tx, nftInfo.NFTId)
				if err != nil {
					log.Error("BuildTransactionInfoFromRequest: Failed to check NFT existence", "err", err, "nftID", nftInfo.NFTId)
					return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: failed to check NFT existence: %w", err)
				}

				if exists {
					// EXECUTION MODE: Token exists, lock it for execution
					log.Info("BuildTransactionInfoFromRequest: NFT exists - EXECUTION MODE", "nftID", nftInfo.NFTId)
					locked, err := w.QueryAndLockForExecution(ctx, tx, req.Initiator, []string{nftInfo.NFTId}, constants.TokenType_NFT, req.Tokens.TransferNFTOwnership)
					if err != nil {
						log.Error("BuildTransactionInfoFromRequest: NFT lock failed", "err", err, "nftID", nftInfo.NFTId)
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: NFT lock failed for %s: %w", nftInfo.NFTId, err)
					}
					if len(locked) == 0 {
						log.Error("BuildTransactionInfoFromRequest: NFT exists but could not be locked", "nftID", nftInfo.NFTId)
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: NFT %s exists but could not be locked", nftInfo.NFTId)
					}
					tok := locked[0]
					log.Info("BuildTransactionInfoFromRequest: NFT locked for execution", "nftID", tok.TokenID, "prevTxID", tok.TransactionID)
					txTokens.NFT = append(txTokens.NFT, &models.TokenInfo{
						TokenID:               tok.TokenID,
						PreviousTransactionID: tok.TransactionID,
						Data:                  nftInfo.Data,
						TokenValue:            tok.TokenValue,
					})
					allLockedIDs = append(allLockedIDs, tok.TokenID)
					totalAmount += tok.TokenValue
				} else {
					// DEPLOYMENT MODE: Token doesn't exist, prepare for deployment
					log.Info("BuildTransactionInfoFromRequest: NFT does not exist - DEPLOYMENT MODE", "nftID", nftInfo.NFTId, "value", nftInfo.Value)
					// Add NFT to transaction tokens (will be deployed during consensus)
					txTokens.NFT = append(txTokens.NFT, &models.TokenInfo{
						TokenID:               nftInfo.NFTId,
						PreviousTransactionID: "", // Genesis - no previous transaction
						Data:                  nftInfo.Data,
						TokenValue:            nftInfo.Value,
					})
					totalAmount += nftInfo.Value
				}
			}
		}

		// SmartContract path - handle both deployment and execution
		if req.HasSmartContract() {
			allSCs := req.GetAllSmartContracts()
			log.Info("BuildTransactionInfoFromRequest: Processing SmartContract tokens", "scCount", len(allSCs))

			for _, scInfo := range allSCs {
				log.Debug("BuildTransactionInfoFromRequest: Checking SC existence", "scID", scInfo.SmartContractId)
				// Check if SC token exists in database
				exists, err := w.TokenExists(ctx, tx, scInfo.SmartContractId)
				if err != nil {
					log.Error("BuildTransactionInfoFromRequest: Failed to check SC existence", "err", err, "scID", scInfo.SmartContractId)
					return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: failed to check SC existence: %w", err)
				}

				if exists {
					// EXECUTION MODE: Token exists, lock it for execution
					log.Info("BuildTransactionInfoFromRequest: SC exists - EXECUTION MODE", "scID", scInfo.SmartContractId)
					locked, err := w.QueryAndLockForExecution(ctx, tx, req.Initiator, []string{scInfo.SmartContractId}, constants.TokenType_SmartContract, false)
					if err != nil {
						log.Error("BuildTransactionInfoFromRequest: SC lock failed", "err", err, "scID", scInfo.SmartContractId)
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: SC lock failed for %s: %w", scInfo.SmartContractId, err)
					}
					if len(locked) == 0 {
						log.Error("BuildTransactionInfoFromRequest: SC exists but could not be locked", "scID", scInfo.SmartContractId)
						return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: SC %s exists but could not be locked", scInfo.SmartContractId)
					}
					tok := locked[0]
					log.Info("BuildTransactionInfoFromRequest: SC locked for execution", "scID", tok.TokenID, "prevTxID", tok.TransactionID)
					txTokens.SmartContract = append(txTokens.SmartContract, &models.TokenInfo{
						TokenID:               tok.TokenID,
						PreviousTransactionID: tok.TransactionID,
						Data:                  scInfo.Data,
						TokenValue:            tok.TokenValue,
					})
					allLockedIDs = append(allLockedIDs, tok.TokenID)
					totalAmount += tok.TokenValue
				} else {
					// DEPLOYMENT MODE: Token doesn't exist, prepare for deployment
					log.Info("BuildTransactionInfoFromRequest: SC does not exist - DEPLOYMENT MODE", "scID", scInfo.SmartContractId, "value", scInfo.Value)
					// Collect committed tokens if SC has value > 0
					if scInfo.Value > 0 {
						log.Info("BuildTransactionInfoFromRequest: Locking RBT tokens for SC committed value", "scID", scInfo.SmartContractId, "value", scInfo.Value)
						// Lock RBT tokens for the smart contract value
						ownedRBTTokens, err := w.LockTokensForSplit(ctx, req.Initiator, scInfo.Value)
						if err != nil {
							log.Error("BuildTransactionInfoFromRequest: Failed to lock RBT for SC committed tokens", "err", err, "scID", scInfo.SmartContractId, "value", scInfo.Value)
							return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: failed to lock RBT for SC committed tokens: %w", err)
						}
						log.Info("BuildTransactionInfoFromRequest: RBT tokens locked for SC commitment", "scID", scInfo.SmartContractId, "tokenCount", len(ownedRBTTokens))

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
						log.Debug("BuildTransactionInfoFromRequest: Committed tokens prepared", "count", len(ownedRBTTokens))
					}

					// Add SC to transaction tokens (will be deployed during consensus)
					txTokens.SmartContract = append(txTokens.SmartContract, &models.TokenInfo{
						TokenID:               scInfo.SmartContractId,
						PreviousTransactionID: "", // Genesis - no previous transaction
						Data:                  scInfo.Data,
						TokenValue:            scInfo.Value,
					})
					totalAmount += scInfo.Value
				}
			}
		}

		// Single batch UPDATE for all locked tokens, then COMMIT
		if len(allLockedIDs) > 0 {
			log.Info("BuildTransactionInfoFromRequest: Updating token status to LOCKED", "tokenCount", len(allLockedIDs))
			_, err = tx.Exec(ctx,
				`UPDATE tokens SET token_status = $1, updated_at = $2 WHERE token_id = ANY($3::text[])`,
				constants.TokenStatus_Locked, time.Now(), allLockedIDs,
			)
			if err != nil {
				log.Error("BuildTransactionInfoFromRequest: Failed to update token status", "err", err, "tokenCount", len(allLockedIDs))
				return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: update status: %w", err)
			}
			log.Debug("BuildTransactionInfoFromRequest: Token status updated to LOCKED")
		}

		log.Debug("BuildTransactionInfoFromRequest: Committing DB transaction")
		if err := tx.Commit(ctx); err != nil {
			log.Error("BuildTransactionInfoFromRequest: Failed to commit transaction", "err", err)
			return nil, 0, fmt.Errorf("BuildTransactionInfoFromRequest: commit: %w", err)
		}
		log.Debug("BuildTransactionInfoFromRequest: DB transaction committed")
	}

	// Set CommittedTokens from SC deployments (will be nil if no SC deployments occurred)
	var committedTokens []*models.TokenInfo
	if len(allCommittedTokens) > 0 {
		committedTokens = allCommittedTokens
		log.Info("BuildTransactionInfoFromRequest: CommittedTokens added for SC deployment", "count", len(allCommittedTokens))
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

	log.Info("BuildTransactionInfoFromRequest: Completed successfully",
		"totalAmount", totalAmount,
		"rbtCount", len(txTokens.RBT),
		"ftCount", len(txTokens.FT),
		"nftCount", len(txTokens.NFT),
		"scCount", len(txTokens.SmartContract),
		"committedTokensCount", len(committedTokens),
		"network", networkMode,
	)

	return txInfo, totalAmount, nil
}
