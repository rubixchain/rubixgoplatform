package wallet

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

type persistenceTokenInput struct {
	TokenID               string
	PreviousTransactionID string
	RoleName              string
	TokenTypeName         string
	TokenValue            float64
}

// BuildPersistencePayload derives tokenchain rows and token states from
// TransactionInfo plus current PostgreSQL state. It performs no writes.
//
// Use this when the caller has finalized transaction payload but does not want
// to manually build TokenChainRows and TokenStates. PersistPostConsensus calls
// this automatically when those request fields are empty.
//
// This function is intentionally limited to code-defined rules already present
// in the repository:
//   - normal transaction tokens derive transfer rows
//   - transaction tokens without PreviousTransactionID derive mint rows
//   - CommittedTokens derive commit rows
//
// It preserves current token_status from the tokens table and updates only the
// fields needed for post-consensus persistence.
func (w *Wallet) BuildPersistencePayload(ctx context.Context, transactionID string, txInfo *models.TransactionInfo, did, executionRole string, transferNFTOwnership bool) ([]models.TokenChain, []models.Token, []string, error) {
	if w == nil {
		return nil, nil, nil, fmt.Errorf("post-consensus persistence: wallet is nil")
	}
	if ctx == nil {
		ctx = w.Ctx
	}
	if transactionID == "" {
		return nil, nil, nil, fmt.Errorf("post-consensus persistence: transaction id is required to derive payload")
	}
	if txInfo == nil {
		return nil, nil, nil, fmt.Errorf("post-consensus persistence: transaction info is required to derive payload")
	}
	if did == "" {
		return nil, nil, nil, fmt.Errorf("post-consensus persistence: did is required to derive payload")
	}
	if !isValidExecutionRole(executionRole) {
		return nil, nil, nil, fmt.Errorf("post-consensus persistence: invalid execution role %q", executionRole)
	}

	inputs, affectedTokens, err := collectPersistenceTokenInputs(txInfo, transferNFTOwnership, executionRole)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(inputs) == 0 {
		return nil, nil, nil, fmt.Errorf("post-consensus persistence: no transaction tokens available to derive payload")
	}

	currentTokens, err := w.readTokensByIDs(ctx, affectedTokens)
	if err != nil {
		return nil, nil, nil, err
	}
	latestRows, err := w.readLatestTokenChainRows(ctx, affectedTokens)
	if err != nil {
		return nil, nil, nil, err
	}

	tokenChains := make([]models.TokenChain, 0, len(inputs))
	tokenStates := make([]models.Token, 0, len(inputs))

	for _, input := range inputs {
		roleID := models.GetTokenRoleID(input.RoleName)
		if roleID <= 0 {
			return nil, nil, nil, fmt.Errorf("post-consensus persistence: unsupported token role %q for token %q", input.RoleName, input.TokenID)
		}

		// For genesis transactions (Deploy, Mint), token doesn't exist yet in the database
		// Skip token state lookup and initialize empty token state
		isGenesis := input.RoleName == constants.TokenRole_Deploy || input.RoleName == constants.TokenRole_Mint
		var currentToken models.Token
		var exists bool

		if !isGenesis {
			// Non-genesis: Token must exist in database (Transfer, Execute, Commit, etc.)
			currentToken, exists = currentTokens[input.TokenID]
			if !exists {
				// Special case: Receiver receiving token for first time (not in their DB yet)
				if executionRole == ExecutionRoleReceiver {
					// Receiver genesis case: token is arriving for the first time.
					// Synthesize a zero-value token so buildDerivedTokenChainRow produces position=0.
					tokenValue := input.TokenValue
					if tokenValue == 0 {
						// Fallback: derive from token ID (only works for RBT tokens)
						if derived, err := util.GetTokenValueFromTokenID(input.TokenID); err == nil && derived > 0 {
							tokenValue = derived
						}
					}
					// NFT tokens can have zero value (collectibles with no monetary value)
					// RBT and SmartContract tokens must have a value > 0
					if tokenValue == 0 && input.TokenTypeName != constants.TokenType_NFT {
						return nil, nil, nil, fmt.Errorf("post-consensus persistence: cannot determine token value for genesis token %q", input.TokenID)
					}
					tokenTypeID := models.GetTokenTypeID(input.TokenTypeName)
					if tokenTypeID <= 0 {
						tokenTypeID = models.GetTokenTypeID(constants.TokenType_RBT)
					}
					currentToken = models.Token{
						TokenID:    input.TokenID,
						TokenValue: tokenValue,
						TokenType:  int16(tokenTypeID),
					}
					// Force position=0: receiver's chain starts fresh for this token.
					input.PreviousTransactionID = ""
				} else {
					return nil, nil, nil, fmt.Errorf("post-consensus persistence: token state for token %q is required to derive payload", input.TokenID)
				}
			}
		} else {
			// Genesis (Deploy/Mint): Token is being created, initialize empty state
			tokenValue := input.TokenValue
			if tokenValue == 0 {
				// Try to derive from token ID
				if derived, err := util.GetTokenValueFromTokenID(input.TokenID); err == nil && derived > 0 {
					tokenValue = derived
				}
			}
			tokenTypeID := models.GetTokenTypeID(input.TokenTypeName)
			if tokenTypeID <= 0 {
				tokenTypeID = models.GetTokenTypeID(constants.TokenType_RBT)
			}
			currentToken = models.Token{
				TokenID:    input.TokenID,
				TokenValue: tokenValue,
				TokenType:  int16(tokenTypeID),
			}
		}

		row, position, err := buildDerivedTokenChainRow(transactionID, currentToken, latestRows[input.TokenID], input, int16(roleID))
		if err != nil {
			return nil, nil, nil, err
		}

		state := currentToken
		if state.TokenValue == 0 {
			tokenValue := input.TokenValue

			if tokenValue == 0 {
				if derived, err := util.GetTokenValueFromTokenID(input.TokenID); err == nil && derived > 0 {
					tokenValue = derived
				}
			}

			if tokenValue == 0 {
				return nil, nil, nil, fmt.Errorf("post-consensus persistence: cannot determine token value for token %q", input.TokenID)
			}

			state.TokenValue = tokenValue
		}
		state.TransactionID = transactionID
		state.LatestPosition = position
		state.LatestRole = int16(roleID)

		// Set token status based on role and execution context
		switch input.RoleName {
		case constants.TokenRole_Deploy:
			// NFT/SC Deployment - token stays with initiator in Deployed status
			if txInfo.Initiator != "" {
				state.DID = txInfo.Initiator
			}
			state.TokenStatus = int16(constants.TokenStatus_Deployed)
		case constants.TokenRole_Execute:
			// SC/NFT Execution - token stays with initiator in Executed status
			if txInfo.Initiator != "" {
				state.DID = txInfo.Initiator
			}
			state.TokenStatus = int16(constants.TokenStatus_Executed)
		default:
			// Transfer, Mint, Commit roles - use existing logic based on execution role
			switch executionRole {
			case ExecutionRoleReceiver:
				if txInfo.Owner != "" {
					state.DID = txInfo.Owner
				}
				state.TokenStatus = int16(constants.TokenStatus_Free)
			case ExecutionRoleInitiator:
				if txInfo.Initiator != "" {
					state.DID = txInfo.Initiator
				}
				// Tokens are leaving the initiator — mark as Transferred so they are no
				// longer counted as available balance. Non-selected locked tokens will be
				// released separately by ReleaseAllLockedRBTTokensForDID.
				state.TokenStatus = int16(constants.TokenStatus_Transferred)
			}
		}

		tokenChains = append(tokenChains, row)
		tokenStates = append(tokenStates, state)
	}

	return tokenChains, tokenStates, affectedTokens, nil
}

func collectPersistenceTokenInputs(txInfo *models.TransactionInfo, transferNFTOwnership bool, executionRole string) ([]persistenceTokenInput, []string, error) {
	seen := make(map[string]struct{})
	inputs := make([]persistenceTokenInput, 0)
	affected := make([]string, 0)

	appendInputs := func(tokens []*models.TokenInfo, roleName string, tokenTypeName string) error {
		for _, tokenInfo := range tokens {
			if tokenInfo == nil {
				return fmt.Errorf("post-consensus persistence: transaction token is nil")
			}
			if tokenInfo.TokenID == "" {
				return fmt.Errorf("post-consensus persistence: transaction token id is empty")
			}
			if _, exists := seen[tokenInfo.TokenID]; exists {
				return fmt.Errorf("post-consensus persistence: duplicate token %q in transaction payload", tokenInfo.TokenID)
			}
			seen[tokenInfo.TokenID] = struct{}{}
			affected = append(affected, tokenInfo.TokenID)
			derivedRoleName := roleName
			if roleName == constants.TokenRole_Transfer && tokenInfo.PreviousTransactionID == "" {
				derivedRoleName = constants.TokenRole_Mint
			}
			inputs = append(inputs, persistenceTokenInput{
				TokenID:               tokenInfo.TokenID,
				PreviousTransactionID: tokenInfo.PreviousTransactionID,
				RoleName:              derivedRoleName,
				TokenTypeName:         tokenTypeName,
				TokenValue:            tokenInfo.TokenValue,
			})
		}
		return nil
	}

	// Process each token type with appropriate role assignment
	if txInfo.Tokens != nil {
		// RBT tokens: Transfer role (becomes Mint for genesis via PreviousTransactionID check in appendInputs)
		if err := appendInputs(txInfo.Tokens.RBT, constants.TokenRole_Transfer, constants.TokenType_RBT); err != nil {
			return nil, nil, err
		}

		// NFT tokens: Check if genesis (deployment), execution, or transfer
		// - If PreviousTransactionID is empty → Deploy (genesis)
		// - If transferNFTOwnership is false → Execute (no ownership change)
		// - If transferNFTOwnership is true → Transfer (ownership change)
		//
		// For receiver role: only include NFT tokens when ownership is being transferred.
		// Deploy and Execute operations are initiator-only — the NFT stays with the
		// initiator. Including them for the receiver in mixed-asset transactions
		// (e.g., RBT + NFT execute) would create phantom NFT records on the receiver node.
		for _, nft := range txInfo.Tokens.NFT {
			if nft == nil {
				return nil, nil, fmt.Errorf("post-consensus persistence: transaction token is nil")
			}
			if nft.TokenID == "" {
				return nil, nil, fmt.Errorf("post-consensus persistence: transaction token id is empty")
			}

			// Receiver should only persist NFT tokens when ownership is being transferred.
			// Deploy and Execute are initiator-only operations — skip for receiver.
			if executionRole == ExecutionRoleReceiver && !transferNFTOwnership {
				continue
			}

			if _, exists := seen[nft.TokenID]; exists {
				return nil, nil, fmt.Errorf("post-consensus persistence: duplicate token %q in transaction payload", nft.TokenID)
			}
			seen[nft.TokenID] = struct{}{}
			affected = append(affected, nft.TokenID)

			// Determine role based on PreviousTransactionID and transferNFTOwnership flag.
			// The flag is authoritative: false=Execute (no ownership change), true=Transfer.
			// This replaces the old Owner==Initiator heuristic which was incorrect in
			// mixed transactions (e.g., RBT transfer + NFT execution) where Owner is
			// the RBT receiver, not the NFT owner.
			var roleName string
			if nft.PreviousTransactionID == "" {
				roleName = constants.TokenRole_Deploy // Genesis - NFT deployment
			} else if !transferNFTOwnership {
				roleName = constants.TokenRole_Execute // NFT execution without ownership change
			} else {
				roleName = constants.TokenRole_Transfer // NFT ownership transfer
			}

			inputs = append(inputs, persistenceTokenInput{
				TokenID:               nft.TokenID,
				PreviousTransactionID: nft.PreviousTransactionID,
				RoleName:              roleName,
				TokenTypeName:         constants.TokenType_NFT,
				TokenValue:            nft.TokenValue,
			})
		}

		// FT tokens: Transfer role
		if err := appendInputs(txInfo.Tokens.FT, constants.TokenRole_Transfer, constants.TokenType_FT); err != nil {
			return nil, nil, err
		}

		// SmartContract tokens: Check if genesis (deployment) or execution
		// - If PreviousTransactionID is empty → Deploy (genesis)
		// - Otherwise → Execute (existing SC)
		//
		// SC tokens are never included for the receiver role. Smart contract
		// deploy and execute operations are strictly initiator-side — the SC
		// token stays with the initiator. In mixed-asset transactions (e.g.,
		// RBT + SC execute), the receiver should only persist the RBT tokens.
		if executionRole != ExecutionRoleReceiver {
			for _, sc := range txInfo.Tokens.SmartContract {
				if sc == nil {
					return nil, nil, fmt.Errorf("post-consensus persistence: transaction token is nil")
				}
				if sc.TokenID == "" {
					return nil, nil, fmt.Errorf("post-consensus persistence: transaction token id is empty")
				}
				if _, exists := seen[sc.TokenID]; exists {
					return nil, nil, fmt.Errorf("post-consensus persistence: duplicate token %q in transaction payload", sc.TokenID)
				}
				seen[sc.TokenID] = struct{}{}
				affected = append(affected, sc.TokenID)

				// Determine role based on PreviousTransactionID
				roleName := constants.TokenRole_Execute // Execute for existing SC
				if sc.PreviousTransactionID == "" {
					roleName = constants.TokenRole_Deploy // Genesis - SC deployment
				}

				inputs = append(inputs, persistenceTokenInput{
					TokenID:               sc.TokenID,
					PreviousTransactionID: sc.PreviousTransactionID,
					RoleName:              roleName,
					TokenTypeName:         constants.TokenType_SmartContract,
					TokenValue:            sc.TokenValue,
				})
			}
		}
	}

	// Committed tokens: Commit role
	if err := appendInputs(txInfo.CommittedTokens, constants.TokenRole_Commit, constants.TokenType_RBT); err != nil {
		return nil, nil, err
	}

	return inputs, affected, nil
}

func buildDerivedTokenChainRow(transactionID string, currentToken models.Token, latestRow *models.TokenChain, input persistenceTokenInput, roleID int16) (models.TokenChain, int64, error) {
	var (
		position   int64
		previousID *string
	)

	switch {
	case latestRow != nil:
		if latestRow.TransactionID == transactionID {
			if latestRow.Role != roleID {
				return models.TokenChain{}, 0, fmt.Errorf("post-consensus persistence: existing tokenchain role mismatch for token %q", input.TokenID)
			}
			return models.TokenChain{
				ID:                    latestRow.ID,
				TokenID:               latestRow.TokenID,
				TransactionID:         latestRow.TransactionID,
				PreviousTransactionID: latestRow.PreviousTransactionID,
				Role:                  latestRow.Role,
				Position:              latestRow.Position,
			}, latestRow.Position, nil
		}
		if input.PreviousTransactionID != "" && latestRow.TransactionID != input.PreviousTransactionID {
			return models.TokenChain{}, 0, fmt.Errorf("post-consensus persistence: previous transaction id mismatch for token %q", input.TokenID)
		}
		position = latestRow.Position + 1
		prev := latestRow.TransactionID
		previousID = &prev
	case input.PreviousTransactionID != "":
		if currentToken.TransactionID != "" && currentToken.TransactionID != input.PreviousTransactionID {
			return models.TokenChain{}, 0, fmt.Errorf("post-consensus persistence: current token transaction mismatch for token %q", input.TokenID)
		}
		position = currentToken.LatestPosition + 1
		prev := input.PreviousTransactionID
		previousID = &prev
	case currentToken.TransactionID != "" && currentToken.TransactionID != transactionID:
		position = currentToken.LatestPosition + 1
		prev := currentToken.TransactionID
		previousID = &prev
	default:
		position = 0
	}

	if position == 0 {
		previousID = nil
	}

	return models.TokenChain{
		TokenID:               input.TokenID,
		TransactionID:         transactionID,
		PreviousTransactionID: previousID,
		Role:                  roleID,
		Position:              position,
	}, position, nil
}

func (w *Wallet) readTokensByIDs(ctx context.Context, tokenIDs []string) (map[string]models.Token, error) {
	rows, err := w.db.Pool().Query(ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens
		 WHERE token_id = ANY($1::text[])`,
		tokenIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("post-consensus persistence: read tokens: %w", err)
	}
	defer rows.Close()

	tokens := make(map[string]models.Token, len(tokenIDs))
	for rows.Next() {
		var token models.Token
		if err := rows.Scan(
			&token.TokenID, &token.ParentTokenID, &token.TokenValue, &token.TokenStatus,
			&token.DID, &token.TransactionID, &token.TokenStateHash, &token.TokenType,
			&token.LatestPosition, &token.LatestRole, &token.CreatedAt, &token.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("post-consensus persistence: scan token: %w", err)
		}
		tokens[token.TokenID] = token
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("post-consensus persistence: stream tokens: %w", err)
	}

	return tokens, nil
}

func (w *Wallet) readLatestTokenChainRows(ctx context.Context, tokenIDs []string) (map[string]*models.TokenChain, error) {
	rows, err := w.db.Pool().Query(ctx,
		`SELECT DISTINCT ON (token_id)
		 id, token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at
		 FROM tokenchain
		 WHERE token_id = ANY($1::text[])
		 ORDER BY token_id, position DESC`,
		tokenIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("post-consensus persistence: read latest tokenchain rows: %w", err)
	}
	defer rows.Close()

	latestRows := make(map[string]*models.TokenChain, len(tokenIDs))
	for rows.Next() {
		var row models.TokenChain
		if err := rows.Scan(
			&row.ID, &row.TokenID, &row.TransactionID, &row.PreviousTransactionID, &row.Role,
			&row.Position, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("post-consensus persistence: scan latest tokenchain row: %w", err)
		}
		rowCopy := row
		latestRows[row.TokenID] = &rowCopy
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("post-consensus persistence: stream latest tokenchain rows: %w", err)
	}

	return latestRows, nil
}
