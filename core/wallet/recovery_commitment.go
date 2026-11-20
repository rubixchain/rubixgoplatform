package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// GetRecoveryPledgedTokens returns the tokens quorumDID is still pledged for
// on behalf of receiverDID, each mapped to the IDs of the pledged
// transactions that cover it.
//
// A token qualifies if an unpledge_sequence_info row owned by quorumDID lists
// it in transaction_tokens and that row's transaction names receiverDID as
// owner and moves the token as an RBT (recovery covers RBTs only). The row is the live pledge: unpledging deletes it, which releases
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
		  AND (t.info->'tokens'->'rbt')::jsonb @> jsonb_build_array(jsonb_build_object('tokenId', tok))
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
func (w *Wallet) GetRecoveryTokenChains(tokenIDs []string) (map[string][]types.RecoveryChainEntry, error) {
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
func collectRecoveryChains(rows pgx.Rows) (map[string][]types.RecoveryChainEntry, error) {
	defer rows.Close()
	chains := make(map[string][]types.RecoveryChainEntry)
	for rows.Next() {
		var (
			tokenID string
			prevTx  *string
			info    *string
			e       types.RecoveryChainEntry
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

// GetFullNodeRecoveryTipChains returns, from the fullnode tables, the full
// tokenchain of every token whose latest transaction names receiverDID as
// owner, in position order. Rows are shaped as in GetRecoveryTokenChains.
func (w *Wallet) GetFullNodeRecoveryTipChains(receiverDID string) (map[string][]types.RecoveryChainEntry, error) {
	rows, err := w.db.Pool().Query(w.Ctx, `
		WITH tips AS (
			SELECT tc.token_id
			FROM fullnode_transactions t
			JOIN fullnode_tokenchain tc ON tc.transaction_id = t.id
			WHERE t.info->>'owner' = $1
			  AND t.info::jsonb->'tokens'->'rbt' @> jsonb_build_array(jsonb_build_object('tokenId', tc.token_id))
			  AND NOT EXISTS (
				SELECT 1 FROM fullnode_tokenchain n
				WHERE n.token_id = tc.token_id AND n.position > tc.position)
		)
		SELECT tc.token_id, tc.transaction_id, tc.previous_transaction_id, tc.position, tc.role, t.info::text
		FROM fullnode_tokenchain tc
		JOIN tips USING (token_id)
		LEFT JOIN fullnode_transactions t ON t.id = tc.transaction_id
		ORDER BY tc.token_id, tc.position
	`, receiverDID)
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeRecoveryTipChains: %w", err)
	}
	chains, err := collectRecoveryChains(rows)
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeRecoveryTipChains: %w", err)
	}
	return chains, nil
}

// GetFullNodeRecoveryChains reads user RBT chains, then their collateral.
// Both reads use the same snapshot while new transactions arrive.
func (w *Wallet) GetFullNodeRecoveryChains(userDID string) (map[string][]types.RecoveryChainEntry, error) {
	tx, err := w.db.Pool().BeginTx(w.Ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeRecoveryChains: begin: %w", err)
	}
	defer tx.Rollback(w.Ctx) //nolint:errcheck

	rows, err := tx.Query(w.Ctx, `
		WITH user_tokens AS (
			SELECT token_id FROM fullnode_rbt WHERE did = $1
			UNION
			SELECT tc.token_id FROM fullnode_tokenchain tc
			JOIN fullnode_transactions t ON t.id = tc.transaction_id
			WHERE t.info->>'owner' = $1
			  AND t.info::jsonb->'tokens'->'rbt' @> jsonb_build_array(jsonb_build_object('tokenId', tc.token_id))
			  AND NOT EXISTS (
				SELECT 1 FROM fullnode_tokenchain n
				WHERE n.token_id = tc.token_id AND n.position > tc.position)
		)
		SELECT tc.token_id, tc.transaction_id, tc.previous_transaction_id, tc.position, tc.role, t.info::text
		FROM fullnode_tokenchain tc JOIN user_tokens USING (token_id)
		LEFT JOIN fullnode_transactions t ON t.id = tc.transaction_id
		ORDER BY tc.token_id, tc.position
	`, userDID)
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeRecoveryChains: read user chains: %w", err)
	}
	chains, err := collectRecoveryChains(rows)
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeRecoveryChains: read user chains: %w", err)
	}

	collateralIDs := make(map[string]bool)
	for _, chain := range chains {
		var info models.TransactionInfo
		if err := json.Unmarshal(chain[len(chain)-1].Info, &info); err != nil {
			continue // The caller reports the broken chain in the excluded list.
		}
		for _, quorum := range info.Quorums {
			if quorum == nil {
				continue
			}
			for _, token := range quorum.Tokens {
				if token != nil && token.TokenID != "" {
					if _, loaded := chains[token.TokenID]; !loaded {
						collateralIDs[token.TokenID] = true
					}
				}
			}
		}
	}
	if len(collateralIDs) > 0 {
		ids := make([]string, 0, len(collateralIDs))
		for id := range collateralIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		rows, err := tx.Query(w.Ctx, `
			SELECT tc.token_id, tc.transaction_id, tc.previous_transaction_id, tc.position, tc.role, t.info::text
			FROM fullnode_tokenchain tc
			LEFT JOIN fullnode_transactions t ON t.id = tc.transaction_id
			WHERE tc.token_id = ANY($1::TEXT[])
			ORDER BY tc.token_id, tc.position
		`, ids)
		if err != nil {
			return nil, fmt.Errorf("GetFullNodeRecoveryChains: read collateral chains: %w", err)
		}
		collateral, err := collectRecoveryChains(rows)
		if err != nil {
			return nil, fmt.Errorf("GetFullNodeRecoveryChains: read collateral chains: %w", err)
		}
		for id, chain := range collateral {
			chains[id] = chain
		}
	}
	if err := tx.Commit(w.Ctx); err != nil {
		return nil, fmt.Errorf("GetFullNodeRecoveryChains: commit: %w", err)
	}
	return chains, nil
}

// SaveRecoveryQuorumCommitment stores the commitment tree a quorum published
// for a recovery request. The tree is needed later to deliver the committed
// chains and to answer challenges.
func (w *Wallet) SaveRecoveryQuorumCommitment(quorumDID string, tree *types.RecoveryCommitmentTree) error {
	data, err := json.Marshal(tree)
	if err != nil {
		return fmt.Errorf("SaveRecoveryQuorumCommitment: %w", err)
	}
	_, err = w.db.Pool().Exec(w.Ctx,
		`INSERT INTO recovery_quorum_commitments (request_id, quorum_did, tree)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (request_id, quorum_did) DO NOTHING`,
		tree.Commitment.RequestID, quorumDID, data,
	)
	if err != nil {
		return fmt.Errorf("SaveRecoveryQuorumCommitment: request %s, quorum %s: %w", tree.Commitment.RequestID, quorumDID, err)
	}
	return nil
}

// GetRecoveryQuorumCommitments returns the commitment trees the quorums on
// this node published for a recovery request.
func (w *Wallet) GetRecoveryQuorumCommitments(requestID string) ([]*types.RecoveryCommitmentTree, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT tree FROM recovery_quorum_commitments WHERE request_id = $1 ORDER BY quorum_did`,
		requestID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetRecoveryQuorumCommitments: request %s: %w", requestID, err)
	}
	defer rows.Close()
	var trees []*types.RecoveryCommitmentTree
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("GetRecoveryQuorumCommitments: scan: %w", err)
		}
		var tree types.RecoveryCommitmentTree
		if err := json.Unmarshal(data, &tree); err != nil {
			return nil, fmt.Errorf("GetRecoveryQuorumCommitments: decode: %w", err)
		}
		trees = append(trees, &tree)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetRecoveryQuorumCommitments: request %s: %w", requestID, err)
	}
	return trees, nil
}

// SaveRecoveryDelivery stores the chains one quorum delivered to the user for
// a recovery request. Each quorum's chains are kept apart and are not merged
// into the wallet, so the user can check them against that quorum's
// commitment. The first delivery from a quorum is kept. It returns false if a
// delivery from that quorum was already stored.
func (w *Wallet) SaveRecoveryDelivery(requestID, quorumDID string, chains map[string][]types.TransactionWithRole) (bool, error) {
	data, err := json.Marshal(chains)
	if err != nil {
		return false, fmt.Errorf("SaveRecoveryDelivery: %w", err)
	}
	tag, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO recovery_deliveries (request_id, quorum_did, chains)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (request_id, quorum_did) DO NOTHING`,
		requestID, quorumDID, data,
	)
	if err != nil {
		return false, fmt.Errorf("SaveRecoveryDelivery: request %s, quorum %s: %w", requestID, quorumDID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// GetRecoveryDeliveries lists the chain deliveries stored for a recovery
// request, one per quorum, sorted by quorum DID.
func (w *Wallet) GetRecoveryDeliveries(requestID string) ([]types.RecoveryDeliverySummary, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT quorum_did, (SELECT count(*) FROM jsonb_object_keys(chains))
		 FROM recovery_deliveries WHERE request_id = $1 ORDER BY quorum_did`,
		requestID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetRecoveryDeliveries: request %s: %w", requestID, err)
	}
	defer rows.Close()
	var deliveries []types.RecoveryDeliverySummary
	for rows.Next() {
		var d types.RecoveryDeliverySummary
		if err := rows.Scan(&d.QuorumDID, &d.TokenCount); err != nil {
			return nil, fmt.Errorf("GetRecoveryDeliveries: scan: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// GetRecoveryCommitmentReport returns the fullnode's audit of a recovery
// request, or nil if it has not run.
func (w *Wallet) GetRecoveryCommitmentReport(requestID string) (*types.RecoveryCommitmentReport, error) {
	var data []byte
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT report FROM recovery_commitment_reports WHERE request_id = $1`, requestID,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetRecoveryCommitmentReport: request %s: %w", requestID, err)
	}
	var report types.RecoveryCommitmentReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("GetRecoveryCommitmentReport: request %s: %w", requestID, err)
	}
	return &report, nil
}

// SaveRecoveryCommitmentReport stores a fullnode's audit of the commitments
// submitted for a recovery request. The first report for a request is kept.
func (w *Wallet) SaveRecoveryCommitmentReport(report *types.RecoveryCommitmentReport) error {
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("SaveRecoveryCommitmentReport: %w", err)
	}
	_, err = w.db.Pool().Exec(w.Ctx,
		`INSERT INTO recovery_commitment_reports (request_id, report)
		 VALUES ($1, $2)
		 ON CONFLICT (request_id) DO NOTHING`,
		report.RequestID, data,
	)
	if err != nil {
		return fmt.Errorf("SaveRecoveryCommitmentReport: request %s: %w", report.RequestID, err)
	}
	return nil
}
