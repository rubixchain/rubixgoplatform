package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
)

// GetRecoveryPledgedTokens returns the tokens quorumDID is still pledged for
// on behalf of receiverDID, each mapped to the IDs of the pledged
// transactions that cover it.
//
// A token qualifies if an unpledge_sequence_info row owned by quorumDID lists
// it in transaction_tokens and that row's transaction names receiverDID as
// owner. The row is the live pledge: unpledging deletes it, which releases
// every token of the transaction at once. A row whose transaction is not in
// the local transactions table cannot be attributed and is skipped.
func (w *Wallet) GetRecoveryPledgedTokens(quorumDID, receiverDID string) (map[string][]string, error) {
	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT tok, u.tx_id
		FROM unpledge_sequence_info u
		JOIN transactions t ON t.id = u.tx_id
		CROSS JOIN LATERAL unnest(u.transaction_tokens) AS tok
		WHERE u.quorum_did = $1
		  AND t.info->>'owner' = $2
		ORDER BY tok, u.tx_id
	`, quorumDID, receiverDID)
	if err != nil {
		return nil, fmt.Errorf("GetRecoveryPledgedTokens: %w", err)
	}
	defer rows.Close()

	pledged := make(map[string][]string)
	for rows.Next() {
		var tokenID, txID string
		if err := rows.Scan(&tokenID, &txID); err != nil {
			return nil, fmt.Errorf("GetRecoveryPledgedTokens: scan: %w", err)
		}
		pledged[tokenID] = append(pledged[tokenID], txID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetRecoveryPledgedTokens: %w", err)
	}
	return pledged, nil
}

// GetRecoveryTokenChains returns the full local tokenchain of each given
// token, joined with each transaction's info, in position order. Tokens with
// no local tokenchain are absent from the result. A tokenchain row whose
// transaction is missing comes back with nil Info, so the caller can treat
// the chain as incomplete.
func (w *Wallet) GetRecoveryTokenChains(tokenIDs []string) (map[string][]recoverycommit.ChainEntry, error) {
	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT tc.token_id, tc.transaction_id, tc.previous_transaction_id, tc.position, tc.role, t.info::text
		FROM tokenchain tc
		LEFT JOIN transactions t ON t.id = tc.transaction_id
		WHERE tc.token_id = ANY($1::TEXT[])
		ORDER BY tc.token_id, tc.position
	`, tokenIDs)
	if err != nil {
		return nil, fmt.Errorf("GetRecoveryTokenChains: %w", err)
	}
	chains, err := collectRecoveryChains(rows)
	if err != nil {
		return nil, fmt.Errorf("GetRecoveryTokenChains: %w", err)
	}
	return chains, nil
}

// collectRecoveryChains groups rows of (token_id, transaction_id,
// previous_transaction_id, position, role, info) by token, keeping row order.
func collectRecoveryChains(rows pgx.Rows) (map[string][]recoverycommit.ChainEntry, error) {
	defer rows.Close()
	chains := make(map[string][]recoverycommit.ChainEntry)
	for rows.Next() {
		var (
			tokenID string
			prevTx  *string
			info    *string
			e       recoverycommit.ChainEntry
		)
		if err := rows.Scan(&tokenID, &e.TransactionID, &prevTx, &e.Position, &e.Role, &info); err != nil {
			return nil, fmt.Errorf("scan tokenchain row: %w", err)
		}
		if prevTx != nil {
			e.PreviousTransactionID = *prevTx
		}
		if info != nil {
			e.Info = []byte(*info)
		}
		chains[tokenID] = append(chains[tokenID], e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tokenchain rows: %w", err)
	}
	return chains, nil
}
