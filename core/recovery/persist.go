package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// executionStatusCommitted is the only status the transaction_units table
// accepts (mirrors the normal post-consensus persist path).
const executionStatusCommitted = "committed"

// PersistRecoveredTransactions writes a page of transaction blobs into the local
// `transactions` table, plus a `transaction_units` row for each transaction the
// recovery DID took part in. It runs in its own committed DB transaction so a
// re-run is safe (ON CONFLICT) and progress survives a crash. Participation is
// derived from each transaction's info JSON (determineRecoveryDIDRole); a
// transaction the DID was not part of yields role "" and no transaction_units row.
func (s *Store) PersistRecoveredTransactions(ctx context.Context, did string, txns []RecoveredTransaction) error {
	if did == "" {
		return fmt.Errorf("PersistRecoveredTransactions: did is empty")
	}
	if len(txns) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = s.w.Ctx
	}

	dbtx, err := s.w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PersistRecoveredTransactions: begin tx: %w", err)
	}
	defer dbtx.Rollback(ctx) //nolint:errcheck

	for i := range txns {
		t := &txns[i]
		if t.ID == "" {
			return fmt.Errorf("PersistRecoveredTransactions: transaction %d has empty id", i)
		}
		if len(t.Info) == 0 {
			return fmt.Errorf("PersistRecoveredTransactions: transaction %q has empty info", t.ID)
		}
		if _, err := dbtx.Exec(ctx, `
			INSERT INTO transactions (id, info, signature, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, t.ID, json.RawMessage(t.Info), json.RawMessage(t.Signature)); err != nil {
			return fmt.Errorf("PersistRecoveredTransactions: insert transactions (tx %q): %w", t.ID, err)
		}

		role, roleErr := determineRecoveryDIDRole(t.Info, did)
		if roleErr != nil {
			// A malformed info blob must not abort the page; skip the
			// transaction_units row (a missing unit row is recoverable).
			s.log.Warn("PersistRecoveredTransactions: parse info JSON failed; skipping transaction_units row",
				"tx", t.ID, "err", roleErr)
			continue
		}
		if role == "" {
			continue
		}
		if _, err := dbtx.Exec(ctx, `
			INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			ON CONFLICT (transaction_id, did) DO NOTHING
		`, t.ID, did, role, executionStatusCommitted); err != nil {
			return fmt.Errorf("PersistRecoveredTransactions: insert transaction_units (tx %q): %w", t.ID, err)
		}
	}

	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("PersistRecoveredTransactions: commit: %w", err)
	}
	return nil
}

// PersistRecoveredToken writes the local state for one recovered token: its
// tokenchain (in position order), the tokens row, and the derived accounting
// tables. It runs once per token after all transactions have been persisted by
// PersistRecoveredTransactions, so the deferred transaction_id foreign keys
// resolve at commit and the tokens row can use the fullnode's global tip
// directly. A re-run is a no-op (ON CONFLICT on every write).
func (s *Store) PersistRecoveredToken(ctx context.Context, did string, token *RecoveredToken) error {
	if token == nil {
		return fmt.Errorf("PersistRecoveredToken: token is required")
	}
	if token.TokenID == "" {
		return fmt.Errorf("PersistRecoveredToken: token id is empty")
	}
	if did == "" {
		return fmt.Errorf("PersistRecoveredToken: did is empty")
	}
	tokenTypeID := models.GetTokenTypeID(token.TokenType)
	if tokenTypeID <= 0 {
		return fmt.Errorf("PersistRecoveredToken: unsupported token type %q (token %q)", token.TokenType, token.TokenID)
	}
	if ctx == nil {
		ctx = s.w.Ctx
	}
	state := &token.CurrentState

	dbtx, err := s.w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PersistRecoveredToken: begin tx: %w", err)
	}
	defer dbtx.Rollback(ctx) //nolint:errcheck

	// 1. Insert the token's chain rows, keeping the fullnode positions and
	// prev_tx_ids. previous_transaction_id is null at position 0 (enforced by the
	// local chk_prev_tx_rules check).
	for i := range token.Chain {
		e := &token.Chain[i]
		if e.TxID == "" {
			return fmt.Errorf("PersistRecoveredToken: chain entry %d has empty tx id (token %q)", i, token.TokenID)
		}
		var prevTxID *string
		if e.PrevTxID != "" {
			p := e.PrevTxID
			prevTxID = &p
		}
		if _, err := dbtx.Exec(ctx, `
			INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			ON CONFLICT (token_id, position) DO NOTHING
		`, token.TokenID, e.TxID, prevTxID, e.Role, e.Position); err != nil {
			return fmt.Errorf("PersistRecoveredToken: insert tokenchain (token %q pos %d): %w", token.TokenID, e.Position, err)
		}
	}

	// 2. Upsert the tokens row from the fullnode's current state. Every
	// referenced transaction is already local, so the chain-tip pointers
	// (transaction_id, latest_position, latest_role) use the global tip directly.
	var parentTokenID *string
	if state.ParentTokenID != "" {
		p := state.ParentTokenID
		parentTokenID = &p
	}
	if _, err := dbtx.Exec(ctx, `
		INSERT INTO tokens (
			token_id, parent_token_id, token_value, token_status, did,
			transaction_id, token_state_hash, token_type,
			latest_position, latest_role, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
			token_value      = EXCLUDED.token_value,
			token_status     = EXCLUDED.token_status,
			did              = EXCLUDED.did,
			transaction_id   = EXCLUDED.transaction_id,
			token_state_hash = EXCLUDED.token_state_hash,
			latest_position  = EXCLUDED.latest_position,
			latest_role      = EXCLUDED.latest_role,
			updated_at       = NOW()
	`,
		token.TokenID,
		parentTokenID,
		state.TokenValue,
		state.TokenStatus,
		did,
		state.TransactionID,
		state.TokenStateHash,
		int16(tokenTypeID),
		state.LatestPosition,
		state.LatestRole,
	); err != nil {
		return fmt.Errorf("PersistRecoveredToken: upsert tokens (token %q): %w", token.TokenID, err)
	}

	// 3. Re-derive token_denom for (did, denomination). RBT only; FT/NFT have
	// their own accounting tables. token_value=0 is skipped (normal flow skips it
	// too), and only Free tokens are counted so the count matches spendable
	// balance (as getrbtbalance does). The count is recomputed, not adjusted.
	rbtTypeID := models.GetTokenTypeID(constants.TokenType_RBT)
	if tokenTypeID == rbtTypeID && state.TokenValue > 0 {
		if _, err := dbtx.Exec(ctx, `
			INSERT INTO token_denom (did, denom, count, created_at, updated_at)
			SELECT $1, $2, COUNT(*), NOW(), NOW()
			FROM tokens
			WHERE did = $1 AND token_value = $2 AND token_type = $3 AND token_status = $4
			ON CONFLICT (did, denom) DO UPDATE SET
				count = EXCLUDED.count,
				updated_at = NOW()
		`, did, state.TokenValue, int16(rbtTypeID), int16(constants.TokenStatus_Free)); err != nil {
			return fmt.Errorf("PersistRecoveredToken: upsert token_denom (did=%q denom=%v): %w", did, state.TokenValue, err)
		}
	}

	// 4. Re-derive fts / ft_tokens for recovered FT pieces so the node can see
	// and spend them. FT identity lives in fts (ft_name, creator_did, ft_count)
	// and ft_tokens (token_id -> ft_id), not in the tokens row. FT token ids
	// encode identity as <ftName>_<creatorDID>_<n>, the same parse the normal
	// receive path uses. ft_count counts only Free pieces and is recomputed.
	ftTypeID := models.GetTokenTypeID(constants.TokenType_FT)
	if tokenTypeID == ftTypeID {
		parts := strings.SplitN(token.TokenID, "_", 3)
		if len(parts) != 3 {
			return fmt.Errorf("PersistRecoveredToken: malformed FT token id %q (want <ftName>_<creatorDID>_<n>)", token.TokenID)
		}
		ftName, creatorDID := parts[0], parts[1]

		var ftID int32
		if err := dbtx.QueryRow(ctx, `
			INSERT INTO fts (ft_name, creator_did, ft_count, created_at, updated_at)
			VALUES ($1, $2, 0, NOW(), NOW())
			ON CONFLICT (ft_name, creator_did) DO UPDATE SET updated_at = NOW()
			RETURNING id
		`, ftName, creatorDID).Scan(&ftID); err != nil {
			return fmt.Errorf("PersistRecoveredToken: upsert fts (%s/%s): %w", ftName, creatorDID, err)
		}

		if _, err := dbtx.Exec(ctx, `
			INSERT INTO ft_tokens (token_id, ft_id, created_at, updated_at)
			VALUES ($1, $2, NOW(), NOW())
			ON CONFLICT (token_id) DO UPDATE SET ft_id = EXCLUDED.ft_id, updated_at = NOW()
		`, token.TokenID, ftID); err != nil {
			return fmt.Errorf("PersistRecoveredToken: insert ft_tokens (%q): %w", token.TokenID, err)
		}

		if _, err := dbtx.Exec(ctx, `
			UPDATE fts SET
				ft_count = (
					SELECT COUNT(*)
					FROM ft_tokens jt
					JOIN tokens t ON t.token_id = jt.token_id
					WHERE jt.ft_id = $1 AND t.token_status = $2
				),
				updated_at = NOW()
			WHERE id = $1
		`, ftID, int16(constants.TokenStatus_Free)); err != nil {
			return fmt.Errorf("PersistRecoveredToken: recompute fts.ft_count (id=%d): %w", ftID, err)
		}
	}

	// 5. Re-derive unpledge_sequence_info for a recovered pledged token so the
	// quorum can release the pledge later. PledgeV2 leaves the tokens row at
	// (status=Pledged, transaction_id=mainTxID, latest_role=Pledge), giving the
	// pledge-to-mainTx link above. The remaining columns (transaction_tokens,
	// epoch) come from the main transaction's info JSON, which is local by now.
	// Only currently-pledged tokens are rebuilt; a released pledge is back at Free
	// and is skipped. pledge_tokens is recomputed for tokens sharing a mainTxID.
	pledgeRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Pledge))
	if state.TokenStatus == int16(constants.TokenStatus_Pledged) && state.LatestRole == pledgeRoleID {
		mainTxID := state.TransactionID

		var infoJSON []byte
		if err := dbtx.QueryRow(ctx,
			`SELECT info FROM transactions WHERE id = $1`, mainTxID,
		).Scan(&infoJSON); err != nil {
			return fmt.Errorf("PersistRecoveredToken: read main tx info for pledge (token %q tx %q): %w", token.TokenID, mainTxID, err)
		}

		transactionTokens, epoch, parseErr := extractPledgeMainTxFields(infoJSON)
		if parseErr != nil {
			// A malformed/legacy info blob must not abort the token's recovery;
			// the pledge row is just not rebuilt.
			s.log.Warn("PersistRecoveredToken: parse main tx info for pledge failed; skipping unpledge_sequence_info",
				"token", token.TokenID, "tx", mainTxID, "err", parseErr)
		} else {
			var pledgeTokens []string
			if err := dbtx.QueryRow(ctx, `
				SELECT COALESCE(array_agg(token_id ORDER BY token_id), '{}')
				FROM tokens
				WHERE did = $1 AND token_status = $2 AND transaction_id = $3 AND latest_role = $4
			`, did, int16(constants.TokenStatus_Pledged), mainTxID, pledgeRoleID).Scan(&pledgeTokens); err != nil {
				return fmt.Errorf("PersistRecoveredToken: recompute pledge_tokens (tx %q): %w", mainTxID, err)
			}

			if _, err := dbtx.Exec(ctx, `
				INSERT INTO unpledge_sequence_info (tx_id, pledge_tokens, epoch, quorum_did, transaction_tokens, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
				ON CONFLICT (tx_id) DO UPDATE SET
					pledge_tokens      = EXCLUDED.pledge_tokens,
					epoch              = EXCLUDED.epoch,
					quorum_did         = EXCLUDED.quorum_did,
					transaction_tokens = EXCLUDED.transaction_tokens,
					updated_at         = NOW()
			`, mainTxID, pledgeTokens, epoch, did, transactionTokens); err != nil {
				return fmt.Errorf("PersistRecoveredToken: upsert unpledge_sequence_info (tx %q): %w", mainTxID, err)
			}
		}
	}

	// 6. Rebuild tokenchain_index from the current tokenchain rows for this token.
	var index []int32
	if err := dbtx.QueryRow(ctx,
		`SELECT COALESCE(array_agg(id ORDER BY position), '{}')
		 FROM tokenchain WHERE token_id = $1`,
		token.TokenID,
	).Scan(&index); err != nil {
		return fmt.Errorf("PersistRecoveredToken: read tokenchain_index (token %q): %w", token.TokenID, err)
	}
	if _, err := dbtx.Exec(ctx, `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
			index = EXCLUDED.index,
			updated_at = NOW()
	`, token.TokenID, index); err != nil {
		return fmt.Errorf("PersistRecoveredToken: upsert tokenchain_index (token %q): %w", token.TokenID, err)
	}

	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("PersistRecoveredToken: commit: %w", err)
	}
	return nil
}

// determineRecoveryDIDRole returns the ExecutionRole the recovery DID played in
// a transaction (from its info JSON), or "" if the DID was not a participant.
// Priority is Initiator > Receiver (Owner) > Quorum, so a self-transfer records
// as Initiator, matching the normal one-row-per-(tx, did) model. A decode error
// is returned so the caller can log and skip instead of failing the persist.
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
		return wallet.ExecutionRoleInitiator, nil
	}
	if info.Owner == did {
		return wallet.ExecutionRoleReceiver, nil
	}
	for i := range info.Quorums {
		if info.Quorums[i].Did == did {
			return wallet.ExecutionRoleQuorum, nil
		}
	}
	return "", nil
}

// extractPledgeMainTxFields parses a pledged token's main-transaction info JSON
// for the two unpledge_sequence_info fields not stored on the token row: the
// transferred tokens and the epoch. The transferred-token set must match what
// the live pledge path stores: quorum_initiator.go builds it from
// Tokens.RBT/FT/NFT/SmartContract (not CommittedTokens) before calling PledgeV2,
// so the rebuilt row lines up with the later CallBackQuorumUnpledge check.
func extractPledgeMainTxFields(infoJSON []byte) ([]string, int, error) {
	if len(infoJSON) == 0 {
		return nil, 0, fmt.Errorf("empty transaction info")
	}
	var info models.TransactionInfo
	if err := json.Unmarshal(infoJSON, &info); err != nil {
		return nil, 0, err
	}
	var transactionTokens []string
	if info.Tokens != nil {
		appendTok := func(toks []*models.TokenInfo) {
			for _, t := range toks {
				if t != nil && t.TokenID != "" {
					transactionTokens = append(transactionTokens, t.TokenID)
				}
			}
		}
		appendTok(info.Tokens.RBT)
		appendTok(info.Tokens.FT)
		appendTok(info.Tokens.NFT)
		appendTok(info.Tokens.SmartContract)
	}
	return transactionTokens, info.Epoch, nil
}
