package core

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/consensus"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// persistRecoveredChains writes the chains delivered for session into the
// user's wallet as full wallet state (Q15): the transactions, tokenchain and
// token rows, as the wallet recovery from a fullnode does. It runs only after
// completion is published, so every delivery matched its commitment. Each
// token is written once. A token is skipped, and logged, if committers
// delivered different chains for it or a transaction's ID does not match its
// info. It returns the number of tokens written and skipped.
func (c *Core) persistRecoveredChains(session *types.RecoverySession) (int, int, error) {
	deliveries, err := c.w.GetRecoveryDeliveryChains(session.RequestID)
	if err != nil {
		return 0, 0, err
	}
	chains, skipped := mergeRecoveryDeliveries(deliveries)
	tokenIDs := make([]string, 0, len(chains))
	for tokenID := range chains {
		tokenIDs = append(tokenIDs, tokenID)
	}
	sort.Strings(tokenIDs)

	persisted := 0
	for _, tokenID := range tokenIDs {
		if err := c.persistRecoveredChain(session.UserDID, tokenID, chains[tokenID]); err != nil {
			skipped[tokenID] = err.Error()
			continue
		}
		persisted++
	}
	for tokenID, reason := range skipped {
		c.log.Error("Recovered token not written to the wallet", "request_txn_id", session.RequestID, "token_id", tokenID, "reason", reason)
	}
	return persisted, len(skipped), nil
}

// mergeRecoveryDeliveries returns one chain per token from the deliveries of
// every committer. Several quorums pledge the same transaction, and every
// fullnode commits all unpledged tokens, so a token can arrive more than
// once; it is kept only if every copy has the same transactions in the same
// order. The others are returned with the reason they were left out.
func mergeRecoveryDeliveries(deliveries map[string]map[string][]types.TransactionWithRole) (map[string][]types.TransactionWithRole, map[string]string) {
	chains := make(map[string][]types.TransactionWithRole)
	skipped := make(map[string]string)
	for _, delivered := range deliveries {
		for tokenID, chain := range delivered {
			if _, bad := skipped[tokenID]; bad {
				continue
			}
			prev, seen := chains[tokenID]
			if !seen {
				chains[tokenID] = chain
				continue
			}
			if !sameTransactionIDs(prev, chain) {
				delete(chains, tokenID)
				skipped[tokenID] = "committers delivered different chains"
			}
		}
	}
	return chains, skipped
}

func sameTransactionIDs(a, b []types.TransactionWithRole) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Tx.ID != b[i].Tx.ID {
			return false
		}
	}
	return true
}

// persistRecoveredChain writes one RBT's chain and its token row, owned by
// userDID and free, with the value and parent derived from the token ID as
// the chain sync does.
func (c *Core) persistRecoveredChain(userDID, tokenID string, chain []types.TransactionWithRole) error {
	if len(chain) == 0 {
		return fmt.Errorf("empty chain")
	}
	rows := make([]wallet.RecoveredChainRow, len(chain))
	for i, entry := range chain {
		var info models.TransactionInfo
		if err := json.Unmarshal(entry.Tx.Info, &info); err != nil {
			return fmt.Errorf("transaction %s: invalid info: %w", entry.Tx.ID, err)
		}
		if err := consensus.TransactionIDIntegrityCheck(entry.Tx.ID, &info); err != nil {
			return err
		}
		rows[i] = wallet.RecoveredChainRow{
			TokenID:       tokenID,
			TransactionID: entry.Tx.ID,
			Role:          entry.Role,
			Position:      int64(i),
			Info:          entry.Tx.Info,
			Signature:     entry.Tx.Signature,
		}
		if i > 0 {
			rows[i].PreviousTransactionID = &chain[i-1].Tx.ID
		}
	}

	value, err := util.GetTokenValueFromTokenID(tokenID)
	if err != nil {
		return fmt.Errorf("token value: %w", err)
	}
	state := &models.Token{
		TokenID:     tokenID,
		TokenValue:  value,
		TokenStatus: int16(constants.TokenStatus_Free),
		DID:         userDID,
	}
	if value != rubixmath.OneFloat() {
		parent, err := util.TokenID(tokenID).GetParentToken()
		if err != nil {
			return fmt.Errorf("parent token: %w", err)
		}
		state.ParentTokenID = pgtype.Text{String: parent, Valid: true}
	}
	return c.w.PersistRecoveredTokenChainPage(c.w.Ctx, userDID, constants.TokenType_RBT, state, rows)
}
