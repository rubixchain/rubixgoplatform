package recovery

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// Store is the recovery feature's data layer. It runs recovery-specific SQL over
// the wallet's exported connection pool and transaction helpers, the same
// pattern the IPFS provider store uses. It does not reach into wallet internals.
type Store struct {
	w   *wallet.Wallet
	log logger.Logger
}

// NewStore returns a recovery Store backed by the given wallet.
func NewStore(w *wallet.Wallet, log logger.Logger) *Store {
	return &Store{w: w, log: log}
}

// TokenChainRow is one row of the owned-token query: a single chain entry
// (structure only, no info/signature blobs) for an owned token, with the token's
// current state repeated on each of its rows. The handler groups consecutive
// rows by token_id into a RecoveredToken.
type TokenChainRow struct {
	// Token identity and current state, repeated on each chain row of the token.
	TokenID        string
	TokenType      string
	DID            string
	TokenStatus    int16
	TokenValue     float64
	TokenStateHash string
	TransactionID  string // token's global-latest tx (current_state)
	LatestPosition int64
	LatestRole     int16
	ParentTokenID  *string // RBT only

	// Chain entry (one per row).
	ChainTxID string
	Position  int64
	PrevTxID  *string
	Role      int16
}

// GetOwnedTokenPage returns one size-limited page of the tokens owned by `did`:
// chain-structure rows across the fullnode state tables (RBT/FT/NFT) joined to
// fullnode_tokenchain, ordered by (token_id, position) and after the given
// cursor. Smart Contract tokens are excluded. Info/signature blobs are not read
// here; they are shared across tokens and served by GetTransactionPage. The
// first request passes an empty cursor (cursorTokenID="", cursorPosition=-1),
// which matches every row.
func (s *Store) GetOwnedTokenPage(
	ctx context.Context,
	did string,
	cursorTokenID string,
	cursorPosition int64,
	limit int,
) ([]TokenChainRow, error) {
	if ctx == nil {
		ctx = s.w.Ctx
	}
	if did == "" {
		return nil, fmt.Errorf("GetOwnedTokenPage: did is required")
	}
	if limit <= 0 {
		return nil, nil
	}

	rows, err := s.w.Pool().Query(ctx, `
		WITH owned AS (
			SELECT token_id, $2::text AS token_type, did, token_status, token_value,
			       token_state_hash, transaction_id, latest_position, latest_role,
			       parent_token_id::text AS parent_token_id
			FROM fullnode_rbt WHERE did = $1
			UNION ALL
			SELECT token_id, $3::text AS token_type, did, token_status, token_value,
			       token_state_hash, transaction_id, latest_position, latest_role,
			       NULL::text AS parent_token_id
			FROM fullnode_ft WHERE did = $1
			UNION ALL
			SELECT token_id, $4::text AS token_type, did, token_status, token_value,
			       token_state_hash, transaction_id, latest_position, latest_role,
			       NULL::text AS parent_token_id
			FROM fullnode_nft WHERE did = $1
		)
		SELECT o.token_id, o.token_type, o.did, o.token_status, o.token_value,
		       o.token_state_hash, o.transaction_id, o.latest_position, o.latest_role,
		       o.parent_token_id,
		       tc.transaction_id, tc.position, tc.previous_transaction_id, tc.role
		FROM owned o
		JOIN fullnode_tokenchain tc ON tc.token_id = o.token_id
		WHERE (o.token_id, tc.position) > ($5, $6)
		ORDER BY o.token_id ASC, tc.position ASC
		LIMIT $7
	`,
		did,
		constants.TokenType_RBT,
		constants.TokenType_FT,
		constants.TokenType_NFT,
		cursorTokenID,
		cursorPosition,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOwnedTokenPage: query: %w", err)
	}
	defer rows.Close()

	var out []TokenChainRow
	for rows.Next() {
		var r TokenChainRow
		if err := rows.Scan(
			&r.TokenID, &r.TokenType, &r.DID, &r.TokenStatus, &r.TokenValue,
			&r.TokenStateHash, &r.TransactionID, &r.LatestPosition, &r.LatestRole,
			&r.ParentTokenID,
			&r.ChainTxID, &r.Position, &r.PrevTxID, &r.Role,
		); err != nil {
			return nil, fmt.Errorf("GetOwnedTokenPage: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetTransactionPage returns one size-limited page of the transaction blobs
// referenced by `did`'s owned tokens' chains, each fullnode_transactions row at
// most once, ordered by id and after cursorTxID. A transaction can move many
// tokens, so serving each row once removes the per-token duplication. The first
// request passes cursorTxID="" (id > ” matches all).
func (s *Store) GetTransactionPage(
	ctx context.Context,
	did string,
	cursorTxID string,
	limit int,
) ([]RecoveredTransaction, error) {
	if ctx == nil {
		ctx = s.w.Ctx
	}
	if did == "" {
		return nil, fmt.Errorf("GetTransactionPage: did is required")
	}
	if limit <= 0 {
		return nil, nil
	}

	rows, err := s.w.Pool().Query(ctx, `
		WITH owned AS (
			SELECT token_id FROM fullnode_rbt WHERE did = $1
			UNION ALL
			SELECT token_id FROM fullnode_ft  WHERE did = $1
			UNION ALL
			SELECT token_id FROM fullnode_nft WHERE did = $1
		),
		tx_ids AS (
			SELECT DISTINCT tc.transaction_id
			FROM fullnode_tokenchain tc
			JOIN owned o ON o.token_id = tc.token_id
		)
		SELECT t.id, t.info, t.signature
		FROM fullnode_transactions t
		JOIN tx_ids ON tx_ids.transaction_id = t.id
		WHERE t.id > $2
		ORDER BY t.id ASC
		LIMIT $3
	`, did, cursorTxID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetTransactionPage: query: %w", err)
	}
	defer rows.Close()

	var out []RecoveredTransaction
	for rows.Next() {
		var tx RecoveredTransaction
		if err := rows.Scan(&tx.ID, &tx.Info, &tx.Signature); err != nil {
			return nil, fmt.Errorf("GetTransactionPage: scan: %w", err)
		}
		out = append(out, tx)
	}
	return out, rows.Err()
}
