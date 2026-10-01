package wallet

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// PersistNFTBurnRequest carries everything needed to record an NFT burn.
type PersistNFTBurnRequest struct {
	// DID of the NFT owner initiating the burn.
	DID string
	// NFTIDs to mark burnt. IDs already in Burnt status should be filtered out
	// by the caller — this function does not re-check.
	NFTIDs []string
	// BurnTransaction is the self-signed, non-consensus transaction record
	// representing the burn.
	BurnTransaction *models.Transactions
}

// PersistNFTBurn atomically records the burn of one or more NFTs.
//
// Unlike an RBT transfer there is no consensus round behind this write: the
// burn is self-signed, so this function is the sole authority for the state
// change. Everything therefore happens inside a single transaction — a partial
// burn would leave an NFT whose token row and token chain disagree.
//
// Tables written:
//   - transactions      — the burn transaction record
//   - tokens            — status Burnt, role Burn, position bumped
//   - tokenchain        — a burn entry linked to the previous chain tip
//   - tokenchain_index  — the new chain entry appended to the index
//   - transaction_units — initiator record for the burn
//
// token_denom is deliberately NOT touched: it tracks RBT denomination counts,
// and NFTs carry no denomination.
func (w *Wallet) PersistNFTBurn(ctx context.Context, req *PersistNFTBurnRequest) error {
	if req == nil {
		return fmt.Errorf("PersistNFTBurn: request is nil")
	}
	if req.DID == "" {
		return fmt.Errorf("PersistNFTBurn: DID is required")
	}
	if req.BurnTransaction == nil || req.BurnTransaction.ID == "" {
		return fmt.Errorf("PersistNFTBurn: burn transaction is required")
	}
	if len(req.NFTIDs) == 0 {
		return fmt.Errorf("PersistNFTBurn: at least one NFT ID is required")
	}

	tx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PersistNFTBurn: failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Insert the burn transaction record first: the tokens.transaction_id FK
	// references it.
	if _, err := tx.Exec(ctx,
		`INSERT INTO transactions (id, info, signature, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`,
		req.BurnTransaction.ID, req.BurnTransaction.Info, req.BurnTransaction.Signature,
	); err != nil {
		return fmt.Errorf("PersistNFTBurn: insert burn transaction: %w", err)
	}

	burnRoleID := models.GetTokenRoleID(constants.TokenRole_Burn)

	for _, nftID := range req.NFTIDs {
		// Mark the token burnt.
		//
		// The accepted statuses include Locked: BuildTransactionInfoFromRequest
		// runs before finalizeNFTBurn and locks every NFT in the request
		// (QueryAndLockForExecution → core/transaction_builder.go:374), so by the
		// time we get here the NFT is ALWAYS Locked under the normal flow. The
		// "is this NFT burnable at all" decision belongs to
		// validateNFTBurnRequest, which runs before that lock is taken and
		// rejects Pledged/already-Burnt tokens.
		//
		// Free/Deployed/Executed stay accepted so a caller that reaches this
		// function without the builder's lock still works.
		cmdTag, updateErr := tx.Exec(ctx, `
			UPDATE tokens
			SET token_status = $2,
			    latest_role = $3,
			    latest_position = latest_position + 1,
			    transaction_id = $4,
			    updated_at = NOW()
			WHERE token_id = $1
			  AND did = $5
			  AND token_status IN ($6, $7, $8, $9)
		`, nftID, int16(constants.TokenStatus_Burnt), int16(burnRoleID),
			req.BurnTransaction.ID, req.DID,
			int16(constants.TokenStatus_Free),
			int16(constants.TokenStatus_Deployed),
			int16(constants.TokenStatus_Executed),
			int16(constants.TokenStatus_Locked),
		)
		if updateErr != nil {
			return fmt.Errorf("PersistNFTBurn: update token %s: %w", nftID, updateErr)
		}
		if cmdTag.RowsAffected() == 0 {
			return fmt.Errorf("PersistNFTBurn: NFT %s was not updated — it may have changed owner or status since validation", nftID)
		}

		// Append a burn entry to the token chain, linking to the current tip.
		var returnedTokenID string
		var returnedChainID int
		row := tx.QueryRow(ctx, `
			INSERT INTO tokenchain (
				token_id,
				transaction_id,
				previous_transaction_id,
				role,
				position
			)
			VALUES (
				$1,
				$2,
				(
					SELECT transaction_id
					FROM tokenchain
					WHERE token_id = $1
					ORDER BY position DESC
					LIMIT 1
				),
				$3,
				COALESCE((
					SELECT position
					FROM tokenchain
					WHERE token_id = $1
					ORDER BY position DESC
					LIMIT 1
				), -1) + 1
			) RETURNING token_id, id`,
			nftID, req.BurnTransaction.ID, burnRoleID,
		)
		if scanErr := row.Scan(&returnedTokenID, &returnedChainID); scanErr != nil {
			return fmt.Errorf("PersistNFTBurn: insert tokenchain entry for NFT %s: %w", nftID, scanErr)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO tokenchain_index (token_id, index)
			VALUES ($1, ARRAY[$2]::INTEGER[])
			ON CONFLICT (token_id)
			DO UPDATE
			SET index = array_append(tokenchain_index.index, $2),
			    updated_at = NOW();
		`, returnedTokenID, returnedChainID); err != nil {
			return fmt.Errorf("PersistNFTBurn: update tokenchain_index for NFT %s: %w", nftID, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (transaction_id, did) DO NOTHING
	`, req.BurnTransaction.ID, req.DID, ExecutionRoleInitiator, transactionUnitStatusCommitted); err != nil {
		return fmt.Errorf("PersistNFTBurn: insert transaction_units: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("PersistNFTBurn: commit: %w", err)
	}

	w.log.Debug("PersistNFTBurn: burn committed",
		"burnTxID", req.BurnTransaction.ID,
		"did", req.DID,
		"nftIDs", req.NFTIDs,
	)

	return nil
}
