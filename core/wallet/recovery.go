package wallet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// ownedTokenRow is the intermediate row shape the recovery query produces.
// Carries the current state of one token owned by the recovering DID; one
// row per owned token (independent of the token's chain depth).
type ownedTokenRow struct {
	TokenID        string
	TokenType      string
	DID            string
	TokenStatus    int16
	TokenValue     float64
	TokenStateHash string
	TransactionID  string
	LatestPosition int64
	LatestRole     int16
	ParentTokenID  *string // RBT only
}

// RecoverableToken is the exported shape returned by ListOwnedTokensByDID.
// One per owned token regardless of chain depth.
type RecoverableToken struct {
	TokenID        string
	TokenType      string
	DID            string
	TokenStatus    int16
	TokenValue     float64
	TokenStateHash string
	TransactionID  string
	LatestPosition int64
	LatestRole     int16
	ParentTokenID  string
}

// ListOwnedTokensByDID returns every token currently held by `did` across
// the three fullnode state tables (RBT / FT / NFT) regardless of token_status.
// Smart Contract tokens are intentionally excluded — SC ownership uses a
// different model and is deferred.
//
// The recovery handler uses this list to assemble the per-token chain
// payload. It is not paginated at the token level; pagination is done at
// the chain-entry level so that pages stay uniformly sized even when one
// token has a huge chain and another has a tiny one.
func (w *Wallet) ListOwnedTokensByDID(ctx context.Context, did string) ([]RecoverableToken, error) {
	if ctx == nil {
		ctx = w.Ctx
	}
	if did == "" {
		return nil, fmt.Errorf("ListOwnedTokensByDID: did is required")
	}

	q := `
		SELECT token_id, token_type, did, token_status, token_value,
		       token_state_hash, transaction_id, latest_position, latest_role,
		       parent_token_id
		FROM (
			SELECT token_id, $2::text AS token_type, did, token_status, token_value,
			       token_state_hash, transaction_id, latest_position, latest_role,
			       parent_token_id::text AS parent_token_id
			FROM fullnode_rbt
			WHERE did = $1
			UNION ALL
			SELECT token_id, $3::text AS token_type, did, token_status, token_value,
			       token_state_hash, transaction_id, latest_position, latest_role,
			       NULL::text AS parent_token_id
			FROM fullnode_ft
			WHERE did = $1
			UNION ALL
			SELECT token_id, $4::text AS token_type, did, token_status, token_value,
			       token_state_hash, transaction_id, latest_position, latest_role,
			       NULL::text AS parent_token_id
			FROM fullnode_nft
			WHERE did = $1
		) AS owned
		ORDER BY token_id ASC
	`

	rows, err := w.db.Pool().Query(ctx, q,
		did,
		constants.TokenType_RBT,
		constants.TokenType_FT,
		constants.TokenType_NFT,
	)
	if err != nil {
		return nil, fmt.Errorf("ListOwnedTokensByDID: query: %w", err)
	}
	defer rows.Close()

	var result []RecoverableToken
	for rows.Next() {
		var r ownedTokenRow
		if err := rows.Scan(
			&r.TokenID, &r.TokenType, &r.DID, &r.TokenStatus, &r.TokenValue,
			&r.TokenStateHash, &r.TransactionID, &r.LatestPosition, &r.LatestRole,
			&r.ParentTokenID,
		); err != nil {
			return nil, fmt.Errorf("ListOwnedTokensByDID: scan: %w", err)
		}
		parent := ""
		if r.ParentTokenID != nil {
			parent = *r.ParentTokenID
		}
		result = append(result, RecoverableToken{
			TokenID:        r.TokenID,
			TokenType:      r.TokenType,
			DID:            r.DID,
			TokenStatus:    r.TokenStatus,
			TokenValue:     r.TokenValue,
			TokenStateHash: r.TokenStateHash,
			TransactionID:  r.TransactionID,
			LatestPosition: r.LatestPosition,
			LatestRole:     r.LatestRole,
			ParentTokenID:  parent,
		})
	}
	return result, rows.Err()
}

// RecoveredChainRow is one (transaction + tokenchain) row pair returned by
// the per-page recovery query. The handler maps these directly to
// RecoveredTransaction wire entries.
type RecoveredChainRow struct {
	TokenID               string          `db:"token_id"`
	TransactionID         string          `db:"transaction_id"`
	Role                  int16           `db:"role"`
	Position              int64           `db:"position"`
	PreviousTransactionID *string         `db:"previous_transaction_id"`
	Info                  json.RawMessage `db:"info"`
	Signature             json.RawMessage `db:"signature"`
}

// GetRecoverableChainPageByCursor returns up to `limit` chain entries (with
// joined transaction info + signature) for the recovering DID, ordered by
// (token_id, position), strictly AFTER the (cursorTokenID, cursorPosition)
// cursor. Per-token thresholds filter further: only rows with
// `position > thresholds[token]` qualify; tokens absent from the map default
// to -1 → full chain.
//
// On the first request the caller passes an empty cursor
// (cursorTokenID="" cursorPosition=-1) which matches every row.
//
// Tuple comparison ((token_id, position) > ($cursor_tok, $cursor_pos))
// replaces LIMIT + OFFSET so the handler can stop emitting rows once a
// per-page byte budget is reached, without losing track of where to resume.
// The (token_id, position) index makes this a direct seek — no scan-and-skip.
func (w *Wallet) GetRecoverableChainPageByCursor(
	ctx context.Context,
	did string,
	thresholds map[string]int64,
	cursorTokenID string,
	cursorPosition int64,
	limit int,
) ([]RecoveredChainRow, error) {
	if ctx == nil {
		ctx = w.Ctx
	}
	if limit <= 0 {
		return nil, nil
	}
	if did == "" {
		return nil, fmt.Errorf("GetRecoverableChainPageByCursor: did is required")
	}
	thresholdTokens, thresholdValues := mapToParallelArrays(thresholds)

	rows, err := w.db.Pool().Query(ctx, `
		WITH owned AS (
			SELECT token_id FROM fullnode_rbt WHERE did = $1
			UNION ALL
			SELECT token_id FROM fullnode_ft  WHERE did = $1
			UNION ALL
			SELECT token_id FROM fullnode_nft WHERE did = $1
		),
		thresholds AS (
			SELECT * FROM unnest($2::text[], $3::bigint[]) AS t(token_id, threshold)
		)
		SELECT tc.token_id, tc.transaction_id, tc.role, tc.position,
		       tc.previous_transaction_id, t.info, t.signature
		FROM fullnode_tokenchain tc
		JOIN fullnode_transactions t ON t.id = tc.transaction_id
		JOIN owned o ON o.token_id = tc.token_id
		LEFT JOIN thresholds th ON th.token_id = tc.token_id
		WHERE tc.position > COALESCE(th.threshold, -1)
		  AND (tc.token_id, tc.position) > ($4, $5)
		ORDER BY tc.token_id ASC, tc.position ASC
		LIMIT $6
	`, did, thresholdTokens, thresholdValues, cursorTokenID, cursorPosition, limit)
	if err != nil {
		return nil, fmt.Errorf("GetRecoverableChainPageByCursor: query: %w", err)
	}
	defer rows.Close()

	var out []RecoveredChainRow
	for rows.Next() {
		var r RecoveredChainRow
		if err := rows.Scan(
			&r.TokenID, &r.TransactionID, &r.Role, &r.Position,
			&r.PreviousTransactionID, &r.Info, &r.Signature,
		); err != nil {
			return nil, fmt.Errorf("GetRecoverableChainPageByCursor: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DetectDivergentRecoveryTokens returns the subset of tokens whose KnownTokens
// claim does not match the fullnode's chain at the claimed position. Same
// semantics as DetectDivergentSyncTokens but scoped to one DID's owned token
// set — divergence on a token the DID doesn't actually own is reported as
// divergent (the client should not have claimed it).
func (w *Wallet) DetectDivergentRecoveryTokens(ctx context.Context, did string, known map[string]types.TokenChainTip) ([]string, error) {
	if ctx == nil {
		ctx = w.Ctx
	}
	if len(known) == 0 {
		return nil, nil
	}
	tokenIDs := make([]string, 0, len(known))
	positions := make([]int64, 0, len(known))
	claimedTxIDs := make([]string, 0, len(known))
	for tokenID, tip := range known {
		if tokenID == "" {
			continue
		}
		tokenIDs = append(tokenIDs, tokenID)
		positions = append(positions, tip.Position)
		claimedTxIDs = append(claimedTxIDs, tip.TransactionID)
	}
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	rows, err := w.db.Pool().Query(ctx, `
		WITH owned AS (
			SELECT token_id FROM fullnode_rbt WHERE did = $1
			UNION ALL
			SELECT token_id FROM fullnode_ft  WHERE did = $1
			UNION ALL
			SELECT token_id FROM fullnode_nft WHERE did = $1
		)
		SELECT input.token_id
		FROM unnest($2::text[], $3::bigint[], $4::text[]) AS input(token_id, position, claimed_tx_id)
		LEFT JOIN owned o ON o.token_id = input.token_id
		LEFT JOIN fullnode_tokenchain tc
		  ON tc.token_id = input.token_id AND tc.position = input.position
		WHERE o.token_id IS NULL
		   OR tc.transaction_id IS NULL
		   OR tc.transaction_id <> input.claimed_tx_id
	`, did, tokenIDs, positions, claimedTxIDs)
	if err != nil {
		return nil, fmt.Errorf("DetectDivergentRecoveryTokens: %w", err)
	}
	defer rows.Close()

	divergent := make([]string, 0)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("DetectDivergentRecoveryTokens scan: %w", err)
		}
		divergent = append(divergent, t)
	}
	return divergent, rows.Err()
}

// mapToParallelArrays converts a map[string]int64 into the two parallel slices
// the unnest pattern expects. Returns single-element ("",-1) placeholders for
// an empty input so the SQL unnest doesn't degenerate to zero rows (which
// would make the LEFT JOIN match nothing). The placeholder token "" can never
// match a real token_id, so it's harmless.
func mapToParallelArrays(m map[string]int64) ([]string, []int64) {
	if len(m) == 0 {
		return []string{""}, []int64{-1}
	}
	keys := make([]string, 0, len(m))
	values := make([]int64, 0, len(m))
	for k, v := range m {
		keys = append(keys, k)
		values = append(values, v)
	}
	return keys, values
}

// ReadLocalKnownState returns a map of token_id -> {position, tx_id} for
// tokens owned by `did` in the local `tokens` table. Used by a normal node
// to build the KnownTokens field of a recovery request, so the fullnode can
// skip entries the client already has and detect chain divergence.
func (w *Wallet) ReadLocalKnownState(ctx context.Context, did string) (map[string]types.TokenChainTip, error) {
	if ctx == nil {
		ctx = w.Ctx
	}
	rows, err := w.db.Pool().Query(ctx,
		`SELECT token_id, transaction_id, latest_position FROM tokens WHERE did = $1 AND transaction_id <> ''`,
		did,
	)
	if err != nil {
		return nil, fmt.Errorf("ReadLocalKnownState: query: %w", err)
	}
	defer rows.Close()
	out := make(map[string]types.TokenChainTip)
	for rows.Next() {
		var tokenID, txID string
		var pos int64
		if err := rows.Scan(&tokenID, &txID, &pos); err != nil {
			return nil, fmt.Errorf("ReadLocalKnownState: scan: %w", err)
		}
		out[tokenID] = types.TokenChainTip{Position: pos, TransactionID: txID}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ReadLocalKnownState: iterate: %w", err)
	}
	return out, nil
}

// determineRecoveryDIDRole inspects a transaction's info JSON and returns
// the ExecutionRole the recovery DID played in that historical transaction,
// or "" if the DID was not a participant.
//
// Priority is Initiator > Receiver (Owner) > Quorum. A DID that appears as
// both Initiator and Owner (self-mint or self-transfer) records as
// Initiator, matching the normal post-consensus flow's one-row-per-(tx, did)
// model.
//
// Decode failure returns an error so the caller can log and skip rather
// than fail the whole persist — a missing transaction_units row is
// recoverable later, but failing the persist would block the entire chain.
func determineRecoveryDIDRole(infoJSON []byte, did string) (string, error) {
	if len(infoJSON) == 0 || did == "" {
		return "", nil
	}
	var info struct {
		Initiator string `json:"initiator"`
		Owner     string `json:"owner"`
		Quorums   []struct {
			Did string `json:"did"`
		} `json:"quorums"`
	}
	if err := json.Unmarshal(infoJSON, &info); err != nil {
		return "", err
	}
	if info.Initiator == did {
		return ExecutionRoleInitiator, nil
	}
	if info.Owner == did {
		return ExecutionRoleReceiver, nil
	}
	for i := range info.Quorums {
		if info.Quorums[i].Did == did {
			return ExecutionRoleQuorum, nil
		}
	}
	return "", nil
}

// PersistRecoveredTokenChainPage atomically writes a page worth of chain
// entries plus the current token state for a single token recovered from a
// fullnode. This replaces the old PersistRecoveredPledgedToken helper and
// supports the multi-tx-per-token recovery model.
//
// Why a dedicated helper (and not PersistPostConsensus): PersistPostConsensus
// enforces strict chain continuity — each new chain row must continue the
// local chain at position = latest+1 with prev_tx_id matching the local tail.
// Recovery from a fullnode doesn't fit that model: we're cloning the
// fullnode's chain verbatim (with its global positions and prev_tx_ids),
// not appending a single transition. Bypassing the continuity check here
// keeps PersistPostConsensus's invariants intact for the normal transfer
// path that other code relies on.
//
// Idempotent: chain rows use ON CONFLICT (token_id, position) DO NOTHING and
// the tokens row uses ON CONFLICT (token_id) DO UPDATE. The caller may call
// this multiple times for the same token across pages — each call appends
// any new chain entries and re-applies the current state (which is the same
// across all pages for a given token).
//
// `did` is the local wallet DID (the one that owns the token).
// `tokenTypeName` is one of constants.TokenType_RBT / _FT / _NFT.
// `chainEntries` are the entries to write for this token on this page, in
// chronological position order.
// `state` carries the fullnode's current state for the token; written to
// the tokens row.
func (w *Wallet) PersistRecoveredTokenChainPage(
	ctx context.Context,
	did string,
	tokenTypeName string,
	state *models.Token,
	chainEntries []RecoveredChainRow,
) error {
	if w == nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: wallet is nil")
	}
	if state == nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: state is required")
	}
	if state.TokenID == "" {
		return fmt.Errorf("PersistRecoveredTokenChainPage: state.TokenID is empty")
	}
	if did == "" {
		return fmt.Errorf("PersistRecoveredTokenChainPage: did is empty")
	}
	tokenTypeID := models.GetTokenTypeID(tokenTypeName)
	if tokenTypeID <= 0 {
		return fmt.Errorf("PersistRecoveredTokenChainPage: unsupported token type %q", tokenTypeName)
	}
	if ctx == nil {
		ctx = w.Ctx
	}

	dbtx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: begin tx: %w", err)
	}
	defer dbtx.Rollback(ctx) //nolint:errcheck

	// 1. Insert each chain entry's underlying transaction row + the
	// tokenchain row verbatim, preserving global positions and prev_tx_ids.
	for i := range chainEntries {
		e := &chainEntries[i]
		if e.TransactionID == "" {
			return fmt.Errorf("PersistRecoveredTokenChainPage: chain entry %d has empty tx id (token %q)", i, state.TokenID)
		}
		if len(e.Info) == 0 {
			return fmt.Errorf("PersistRecoveredTokenChainPage: chain entry %d has empty Info (token %q tx %q)", i, state.TokenID, e.TransactionID)
		}
		if _, err := dbtx.Exec(ctx, `
			INSERT INTO transactions (id, info, signature, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, e.TransactionID, e.Info, e.Signature); err != nil {
			return fmt.Errorf("PersistRecoveredTokenChainPage: insert transactions (tx %q): %w", e.TransactionID, err)
		}
		if _, err := dbtx.Exec(ctx, `
			INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			ON CONFLICT (token_id, position) DO NOTHING
		`, state.TokenID, e.TransactionID, e.PreviousTransactionID, e.Role, e.Position); err != nil {
			return fmt.Errorf("PersistRecoveredTokenChainPage: insert tokenchain (tx %q pos %d): %w", e.TransactionID, e.Position, err)
		}

		// Re-derive the DID's role in this historical transaction by inspecting
		// the transaction's info JSON. Insert only when the recovery DID was
		// actually a participant (Initiator / Owner / one of Quorums) — chain
		// entries from before the DID acquired the token are silently skipped
		// because the row would misrepresent participation.
		//
		// Decode failures are logged but do NOT fail the persist: a missing
		// transaction_units row is recoverable; an aborted persist is not.
		role, roleErr := determineRecoveryDIDRole(e.Info, did)
		if roleErr != nil {
			w.log.Warn("PersistRecoveredTokenChainPage: parse info JSON failed; skipping transaction_units row",
				"token", state.TokenID, "tx", e.TransactionID, "err", roleErr)
		} else if role != "" {
			if _, err := dbtx.Exec(ctx, `
				INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, NOW(), NOW())
				ON CONFLICT (transaction_id, did) DO NOTHING
			`, e.TransactionID, did, role, transactionUnitStatusCommitted); err != nil {
				return fmt.Errorf("PersistRecoveredTokenChainPage: insert transaction_units (tx %q): %w", e.TransactionID, err)
			}
		}
	}

	// 2. Resolve the local chain tip for this token AFTER the inserts above.
	// We must point the tokens row at a transaction that actually exists in
	// the local `transactions` table — otherwise the tokens.transaction_id
	// FK fails. The fullnode's "global latest" tx might not have arrived yet
	// (it'll come on a later page). So we use the highest-position chain row
	// currently in the local tokenchain, which is guaranteed to have its tx
	// already inserted (either by this page's loop above or a previous page).
	var localLatestTxID string
	var localLatestPosition int64
	var localLatestRole int16
	if err := dbtx.QueryRow(ctx, `
		SELECT transaction_id, position, role
		FROM tokenchain
		WHERE token_id = $1
		ORDER BY position DESC
		LIMIT 1
	`, state.TokenID).Scan(&localLatestTxID, &localLatestPosition, &localLatestRole); err != nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: read local chain tip (token %q): %w", state.TokenID, err)
	}

	// 3. Upsert the tokens row.
	//
	// Chain-tip pointers (transaction_id, latest_position, latest_role) use
	// the LOCAL chain tip just resolved — never the fullnode's global tip —
	// so the tokens.transaction_id FK is always satisfied. As later pages
	// arrive and add chain entries, those pointers advance toward the global
	// tip; by the time the last page lands they match the fullnode's view.
	//
	// Status / value / DID / parent / state_hash come from the fullnode's
	// authoritative current_state — those don't depend on chain depth.
	// Re-applying on every page is idempotent.
	if _, err := dbtx.Exec(ctx, `
		INSERT INTO tokens (
			token_id, parent_token_id, token_value, token_status, did,
			transaction_id, token_state_hash, token_type,
			latest_position, latest_role, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
			token_value     = EXCLUDED.token_value,
			token_status    = EXCLUDED.token_status,
			did             = EXCLUDED.did,
			transaction_id  = EXCLUDED.transaction_id,
			token_state_hash = EXCLUDED.token_state_hash,
			latest_position = EXCLUDED.latest_position,
			latest_role     = EXCLUDED.latest_role,
			updated_at      = NOW()
	`,
		state.TokenID,
		state.ParentTokenID,
		state.TokenValue,
		state.TokenStatus,
		did,
		localLatestTxID,
		state.TokenStateHash,
		int16(tokenTypeID),
		localLatestPosition,
		localLatestRole,
	); err != nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: upsert tokens (token %q): %w", state.TokenID, err)
	}

	// Re-derive token_denom for (did, this token's denomination).
	// Same self-derive pattern as tokenchain_index — always reflects the
	// current tokens table, idempotent across multiple page calls for the
	// same denom. Only meaningful for RBT; NFT/FT/SC have their own
	// accounting tables (fts / ft_tokens). Tokens with token_value=0 are
	// excluded both because the existing normal-flow accounting skips them
	// and to keep token_denom keyed on real denominations only.
	rbtTypeID := models.GetTokenTypeID(constants.TokenType_RBT)
	if tokenTypeID == rbtTypeID && state.TokenValue > 0 {
		if _, err := dbtx.Exec(ctx, `
			INSERT INTO token_denom (did, denom, count, created_at, updated_at)
			SELECT $1, $2, COUNT(*), NOW(), NOW()
			FROM tokens
			WHERE did = $1 AND token_value = $2 AND token_type = $3
			ON CONFLICT (did, denom) DO UPDATE SET
				count = EXCLUDED.count,
				updated_at = NOW()
		`, did, state.TokenValue, int16(rbtTypeID)); err != nil {
			return fmt.Errorf("PersistRecoveredTokenChainPage: upsert token_denom (did=%q denom=%v): %w", did, state.TokenValue, err)
		}
	}

	// 4. Rebuild tokenchain_index from the current tokenchain rows for this
	// token. Done on every call — perfect would be once at end of recovery,
	// but the cost is bounded by the chain depth and keeps the helper self-
	// contained (no separate "finalise" step the caller must remember).
	var index []int32
	if err := dbtx.QueryRow(ctx,
		`SELECT COALESCE(array_agg(id ORDER BY position), '{}')
		 FROM tokenchain WHERE token_id = $1`,
		state.TokenID,
	).Scan(&index); err != nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: read tokenchain_index (token %q): %w", state.TokenID, err)
	}
	if _, err := dbtx.Exec(ctx, `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
			index = EXCLUDED.index,
			updated_at = NOW()
	`, state.TokenID, index); err != nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: upsert tokenchain_index (token %q): %w", state.TokenID, err)
	}

	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("PersistRecoveredTokenChainPage: commit: %w", err)
	}
	return nil
}
