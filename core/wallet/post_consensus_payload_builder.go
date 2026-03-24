package wallet

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

type persistenceTokenInput struct {
	TokenID               string
	PreviousTransactionID string
	RoleName              string
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
func (w *Wallet) BuildPersistencePayload(ctx context.Context, transactionID string, txInfo *models.TransactionInfo, did, executionRole string) ([]models.TokenChain, []models.Token, []string, error) {
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

	inputs, affectedTokens, err := collectPersistenceTokenInputs(txInfo)
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
		currentToken, exists := currentTokens[input.TokenID]
		if !exists {
			return nil, nil, nil, fmt.Errorf("post-consensus persistence: token state for token %q is required to derive payload", input.TokenID)
		}

		roleID := models.GetTokenRoleID(input.RoleName)
		if roleID <= 0 {
			return nil, nil, nil, fmt.Errorf("post-consensus persistence: unsupported token role %q for token %q", input.RoleName, input.TokenID)
		}

		row, position, err := buildDerivedTokenChainRow(transactionID, currentToken, latestRows[input.TokenID], input, int16(roleID))
		if err != nil {
			return nil, nil, nil, err
		}

		state := currentToken
		state.TransactionID = transactionID
		state.LatestPosition = position
		state.LatestRole = int16(roleID)
		switch executionRole {
		case ExecutionRoleReceiver:
			if txInfo.Owner != "" {
				state.DID = txInfo.Owner
			}
		case ExecutionRoleInitiator:
			if txInfo.Initiator != "" {
				state.DID = txInfo.Initiator
			}
		}

		tokenChains = append(tokenChains, row)
		tokenStates = append(tokenStates, state)
	}

	return tokenChains, tokenStates, affectedTokens, nil
}

func collectPersistenceTokenInputs(txInfo *models.TransactionInfo) ([]persistenceTokenInput, []string, error) {
	seen := make(map[string]struct{})
	inputs := make([]persistenceTokenInput, 0)
	affected := make([]string, 0)

	appendInputs := func(tokens []*models.TokenInfo, roleName string) error {
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
			})
		}
		return nil
	}

	if txInfo.Tokens != nil {
		if err := appendInputs(txInfo.Tokens.RBT, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
		if err := appendInputs(txInfo.Tokens.NFT, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
		if err := appendInputs(txInfo.Tokens.FT, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
		if err := appendInputs(txInfo.Tokens.SmartContract, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
	}
	if err := appendInputs(txInfo.CommittedTokens, constants.TokenRole_Commit); err != nil {
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
