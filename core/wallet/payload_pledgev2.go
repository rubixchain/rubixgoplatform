package wallet

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// Deprecated: BuildPledgePayload is unused under the V2 single-transaction
// architecture. PledgeV2 now performs inline SQL without building a payload.
// Retained for reference only.
//
// BuildPledgePayload derives the tokenchain rows and token states required to
// persist a pledge transaction. It reads current token state from the DB (no
// writes) and builds rows that transition each token from LOCKED → PLEDGED
// with role = PLEDGE (8).
//
// The caller is responsible for computing pledgeTxID = SHA3_256(txInfo) before
// calling this function. Both the tokenchain row and the token state returned
// will carry pledgeTxID as their transaction_id.
func (w *Wallet) BuildPledgePayload(
	ctx context.Context,
	pledgeTxID string,
	pledgeTxInfo *models.TransactionInfo,
	quorumDID string,
) ([]models.TokenChain, []models.Token, []string, error) {
	if pledgeTxInfo == nil || pledgeTxInfo.Tokens == nil {
		return nil, nil, nil, fmt.Errorf("BuildPledgePayload: pledgeTxInfo or its Tokens field is nil")
	}
	if pledgeTxID == "" {
		return nil, nil, nil, fmt.Errorf("BuildPledgePayload: pledgeTxID is required")
	}
	if quorumDID == "" {
		return nil, nil, nil, fmt.Errorf("BuildPledgePayload: quorumDID is required")
	}

	pledgeRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Pledge))
	if pledgeRoleID <= 0 {
		return nil, nil, nil, fmt.Errorf("BuildPledgePayload: unrecognised token role %q", constants.TokenRole_Pledge)
	}

	tokenIDs := make([]string, 0, len(pledgeTxInfo.Tokens.RBT))
	for _, t := range pledgeTxInfo.Tokens.RBT {
		if t == nil {
			return nil, nil, nil, fmt.Errorf("BuildPledgePayload: nil TokenInfo in pledgeTxInfo.Tokens.RBT")
		}
		tokenIDs = append(tokenIDs, t.TokenID)
	}
	if len(tokenIDs) == 0 {
		return nil, nil, nil, fmt.Errorf("BuildPledgePayload: no RBT tokens in pledgeTxInfo")
	}

	latestRows, err := w.readLatestTokenChainRows(ctx, tokenIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("BuildPledgePayload: read latest tokenchain rows: %w", err)
	}
	currentTokens, err := w.readTokensByIDs(ctx, tokenIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("BuildPledgePayload: read current token states: %w", err)
	}

	tokenChainRows := make([]models.TokenChain, 0, len(tokenIDs))
	tokenStates := make([]models.Token, 0, len(tokenIDs))

	for _, token := range pledgeTxInfo.Tokens.RBT {
		latestRow, ok := latestRows[token.TokenID]
		if !ok || latestRow == nil {
			return nil, nil, nil, fmt.Errorf("BuildPledgePayload: no tokenchain entry for token %q — token must exist before pledge", token.TokenID)
		}

		position := latestRow.Position + 1
		prevTx := latestRow.TransactionID
		previousID := &prevTx

		tokenChainRows = append(tokenChainRows, models.TokenChain{
			TokenID:               token.TokenID,
			TransactionID:         pledgeTxID,
			PreviousTransactionID: previousID,
			Role:                  pledgeRoleID,
			Position:              position,
		})

		state := currentTokens[token.TokenID]
		state.TransactionID = pledgeTxID
		state.TokenStatus = int16(constants.TokenStatus_Pledged)
		state.DID = quorumDID
		state.LatestPosition = position
		state.LatestRole = pledgeRoleID
		tokenStates = append(tokenStates, state)
	}

	return tokenChainRows, tokenStates, tokenIDs, nil
}

// Deprecated: BuildUnpledgePayload is unused under the V2 single-transaction
// architecture. UnpledgeV2 now performs an atomic UPDATE without building a payload.
// Retained for reference only.
//
// BuildUnpledgePayload derives the tokenchain rows and token states required to
// persist a single-token unpledge transaction. It reads current token state
// from the DB (no writes) and builds rows that transition the token from
// PLEDGED → FREE with role = UNPLEDGE (9).
//
// unpledgeTxID must equal SHA3_256(SerializeTransactionInfo(unpledgeTxInfo)).
// The caller is responsible for computing this before calling BuildUnpledgePayload.
func (w *Wallet) BuildUnpledgePayload(
	ctx context.Context,
	unpledgeTxID string,
	unpledgeTxInfo *models.TransactionInfo,
	quorumDID string,
) ([]models.TokenChain, []models.Token, []string, error) {
	if unpledgeTxInfo == nil || unpledgeTxInfo.Tokens == nil {
		return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: unpledgeTxInfo or its Tokens field is nil")
	}
	if unpledgeTxID == "" {
		return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: unpledgeTxID is required")
	}
	if quorumDID == "" {
		return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: quorumDID is required")
	}

	unpledgeRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Unpledge))
	if unpledgeRoleID <= 0 {
		return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: unrecognised token role %q", constants.TokenRole_Unpledge)
	}

	tokenIDs := make([]string, 0, len(unpledgeTxInfo.Tokens.RBT))
	for _, t := range unpledgeTxInfo.Tokens.RBT {
		if t == nil {
			return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: nil TokenInfo in unpledgeTxInfo.Tokens.RBT")
		}
		tokenIDs = append(tokenIDs, t.TokenID)
	}
	if len(tokenIDs) == 0 {
		return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: no RBT tokens in unpledgeTxInfo")
	}

	latestRows, err := w.readLatestTokenChainRows(ctx, tokenIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: read latest tokenchain rows: %w", err)
	}
	currentTokens, err := w.readTokensByIDs(ctx, tokenIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: read current token states: %w", err)
	}

	tokenChainRows := make([]models.TokenChain, 0, len(tokenIDs))
	tokenStates := make([]models.Token, 0, len(tokenIDs))

	for _, token := range unpledgeTxInfo.Tokens.RBT {
		latestRow, ok := latestRows[token.TokenID]
		if !ok || latestRow == nil {
			return nil, nil, nil, fmt.Errorf("BuildUnpledgePayload: no tokenchain entry for token %q — token must have been pledged", token.TokenID)
		}

		position := latestRow.Position + 1
		prevTx := latestRow.TransactionID
		previousID := &prevTx

		tokenChainRows = append(tokenChainRows, models.TokenChain{
			TokenID:               token.TokenID,
			TransactionID:         unpledgeTxID,
			PreviousTransactionID: previousID,
			Role:                  unpledgeRoleID,
			Position:              position,
		})

		state := currentTokens[token.TokenID]
		state.TransactionID = unpledgeTxID
		state.TokenStatus = int16(constants.TokenStatus_Free)
		state.DID = quorumDID
		state.LatestPosition = position
		state.LatestRole = unpledgeRoleID
		tokenStates = append(tokenStates, state)
	}

	return tokenChainRows, tokenStates, tokenIDs, nil
}
