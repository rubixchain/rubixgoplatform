package wallet

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
)

// scanToken scans a single row from a token query into a wallet.Token.
// Uses *string and *int16 for nullable columns (parent_token_id, latest_role)
// to avoid importing pgtype in the wallet package.
func scanToken(row interface{ Scan(dest ...interface{}) error }) (Token, error) {
	var tok Token
	var parentTokenID *string
	var latestRole *int16

	err := row.Scan(
		&tok.TokenID, &parentTokenID, &tok.TokenValue, &tok.TokenStatus,
		&tok.DID, &tok.TransactionID, &tok.TokenStateHash, &tok.TokenType,
		&tok.LatestPosition, &latestRole, &tok.CreatedAt, &tok.UpdatedAt,
	)
	if err != nil {
		return Token{}, err
	}

	if parentTokenID != nil {
		tok.ParentTokenID = *parentTokenID
	}
	if latestRole != nil {
		tok.LatestRole = *latestRole
	}

	return tok, nil
}

// ── Tx-accepting query helpers ─────────────────────────────────────────
// These run SELECT ... FOR UPDATE within a caller-managed transaction.
// They do NOT update token_status or commit — the caller is responsible.

// QueryAndLockFTs selects and locks FT tokens within an existing transaction.
// Joins with the fts table to filter by ft_name and creator_did.
// Returns exactly `count` tokens or an error.
func (w *Wallet) QueryAndLockFTs(ctx context.Context, tx pgx.Tx, ownerDID string, ftName string, creatorDID string, count int) ([]Token, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.token_id, t.parent_token_id, t.token_value, t.token_status,
		       t.did, t.transaction_id, t.token_state_hash, t.token_type,
		       t.latest_position, t.latest_role, t.created_at, t.updated_at
		FROM tokens t
		INNER JOIN fts f ON f.id = t.token_id
		WHERE t.token_type = (SELECT id FROM token_type WHERE name = $1)
		  AND t.did = $2
		  AND t.token_status = $3
		  AND f.ft_name = $4
		  AND f.creator_did = $5
		ORDER BY t.token_id
		FOR UPDATE OF t
	`, constants.TokenType_FT, ownerDID, constants.TokenStatus_Free, ftName, creatorDID)
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockFTs: query: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		tok, err := scanToken(rows)
		if err != nil {
			return nil, fmt.Errorf("QueryAndLockFTs: scan: %w", err)
		}
		tokens = append(tokens, tok)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("QueryAndLockFTs: rows: %w", err)
	}

	if len(tokens) < count {
		return nil, fmt.Errorf("QueryAndLockFTs: insufficient FTs: have %d, need %d for ft_name=%s creator=%s",
			len(tokens), count, ftName, creatorDID)
	}

	return tokens[:count], nil
}

// QueryAndLockByIDs selects and locks tokens by their IDs within an existing transaction.
// tokenIDs must be pre-sorted by the caller for deadlock prevention.
// Returns all matched tokens or an error if any are missing/not-free/not-owned.
func (w *Wallet) QueryAndLockByIDs(ctx context.Context, tx pgx.Tx, ownerDID string, tokenIDs []string, tokenTypeName string) ([]Token, error) {
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT t.token_id, t.parent_token_id, t.token_value, t.token_status,
		       t.did, t.transaction_id, t.token_state_hash, t.token_type,
		       t.latest_position, t.latest_role, t.created_at, t.updated_at
		FROM tokens t
		WHERE t.token_id = ANY($1::text[])
		  AND t.did = $2
		  AND t.token_type = (SELECT id FROM token_type WHERE name = $3)
		  AND t.token_status = $4
		ORDER BY t.token_id
		FOR UPDATE OF t
	`, tokenIDs, ownerDID, tokenTypeName, constants.TokenStatus_Free)
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockByIDs(%s): query: %w", tokenTypeName, err)
	}
	defer rows.Close()

	var locked []Token
	for rows.Next() {
		tok, err := scanToken(rows)
		if err != nil {
			return nil, fmt.Errorf("QueryAndLockByIDs(%s): scan: %w", tokenTypeName, err)
		}
		locked = append(locked, tok)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("QueryAndLockByIDs(%s): rows: %w", tokenTypeName, err)
	}

	if len(locked) != len(tokenIDs) {
		foundSet := make(map[string]bool, len(locked))
		for _, tok := range locked {
			foundSet[tok.TokenID] = true
		}
		var missing []string
		for _, id := range tokenIDs {
			if !foundSet[id] {
				missing = append(missing, id)
			}
		}
		return nil, fmt.Errorf("QueryAndLockByIDs(%s): tokens not found, not owned by %s, or already locked: %v",
			tokenTypeName, ownerDID, missing)
	}

	return locked, nil
}

// ── Self-contained wrappers ────────────────────────────────────────────
// These open their own transaction, lock tokens, update status, and commit.
// Use these when locking a single asset type independently.

// lockTokensByIDs is the internal batch lock helper. Opens a DB transaction,
// locks all tokens matching the given IDs + type + owner + free status,
// validates all were found, updates status to Locked, and commits.
func (w *Wallet) lockTokensByIDs(ctx context.Context, ownerDID string, tokenIDs []string, tokenTypeName string, label string) ([]Token, error) {
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	sort.Strings(tokenIDs)

	tx, err := w.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", label, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("%s: set lock_timeout: %w", label, err)
	}

	locked, err := w.QueryAndLockByIDs(ctx, tx, ownerDID, tokenIDs, tokenTypeName)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE tokens SET token_status = $1, updated_at = $2 WHERE token_id = ANY($3::text[])`,
		constants.TokenStatus_Locked, time.Now(), tokenIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: update status: %w", label, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("%s: commit: %w", label, err)
	}

	return locked, nil
}

// LockFTTokens selects and locks FT tokens matching ft_name and creator_did.
// Self-contained: opens its own transaction.
func (w *Wallet) LockFTTokens(ctx context.Context, ownerDID string, ftName string, creatorDID string, count int) ([]Token, error) {
	tx, err := w.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("LockFTTokens: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("LockFTTokens: set lock_timeout: %w", err)
	}

	selected, err := w.QueryAndLockFTs(ctx, tx, ownerDID, ftName, creatorDID, count)
	if err != nil {
		return nil, err
	}

	tokenIDs := make([]string, len(selected))
	for i, tok := range selected {
		tokenIDs[i] = tok.TokenID
	}

	_, err = tx.Exec(ctx,
		`UPDATE tokens SET token_status = $1, updated_at = $2 WHERE token_id = ANY($3::text[])`,
		constants.TokenStatus_Locked, time.Now(), tokenIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("LockFTTokens: update status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("LockFTTokens: commit: %w", err)
	}

	return selected, nil
}

// LockNFTTokens locks NFT tokens by IDs. Self-contained.
func (w *Wallet) LockNFTTokens(ctx context.Context, ownerDID string, tokenIDs []string) ([]Token, error) {
	return w.lockTokensByIDs(ctx, ownerDID, tokenIDs, constants.TokenType_NFT, "LockNFTTokens")
}

// LockSmartContractTokens locks SC tokens by IDs. Self-contained.
func (w *Wallet) LockSmartContractTokens(ctx context.Context, ownerDID string, tokenIDs []string) ([]Token, error) {
	return w.lockTokensByIDs(ctx, ownerDID, tokenIDs, constants.TokenType_SmartContract, "LockSmartContractTokens")
}

// LockNFTToken locks a single NFT token. Convenience wrapper.
func (w *Wallet) LockNFTToken(ctx context.Context, ownerDID string, tokenID string) (Token, error) {
	tokens, err := w.LockNFTTokens(ctx, ownerDID, []string{tokenID})
	if err != nil {
		return Token{}, err
	}
	return tokens[0], nil
}

// LockSmartContractToken locks a single SC token. Convenience wrapper.
func (w *Wallet) LockSmartContractToken(ctx context.Context, ownerDID string, tokenID string) (Token, error) {
	tokens, err := w.LockSmartContractTokens(ctx, ownerDID, []string{tokenID})
	if err != nil {
		return Token{}, err
	}
	return tokens[0], nil
}
