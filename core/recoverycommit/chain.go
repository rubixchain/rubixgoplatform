package recoverycommit

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// ChainEntry is one tokenchain row joined with its transaction's info JSON.
// Info is nil when the tokenchain row points at a transaction the node does
// not have.
type ChainEntry struct {
	TransactionID         string
	PreviousTransactionID string // "" at position 0
	Position              int64
	Role                  int16
	Info                  json.RawMessage
}

// TransactionIDs returns the chain's transaction IDs in the order given.
func TransactionIDs(chain []ChainEntry) []string {
	ids := make([]string, len(chain))
	for i, e := range chain {
		ids[i] = e.TransactionID
	}
	return ids
}

// ValidateChain checks that chain is complete and linked: positions 0..n-1,
// each entry points to the previous one, no repeated transaction, and every
// transaction's info is present locally.
func ValidateChain(chain []ChainEntry) error {
	if len(chain) == 0 {
		return fmt.Errorf("empty tokenchain")
	}
	seen := make(map[string]struct{}, len(chain))
	for i, e := range chain {
		if e.Position != int64(i) {
			return fmt.Errorf("position gap: entry %d has position %d", i, e.Position)
		}
		if e.TransactionID == "" {
			return fmt.Errorf("empty transaction ID at position %d", i)
		}
		if _, dup := seen[e.TransactionID]; dup {
			return fmt.Errorf("transaction %s repeated at position %d", e.TransactionID, i)
		}
		seen[e.TransactionID] = struct{}{}
		if i == 0 {
			if e.PreviousTransactionID != "" {
				return fmt.Errorf("position 0 has previous transaction %s", e.PreviousTransactionID)
			}
		} else if e.PreviousTransactionID != chain[i-1].TransactionID {
			return fmt.Errorf("broken link at position %d: previous %q, expected %q",
				i, e.PreviousTransactionID, chain[i-1].TransactionID)
		}
		if len(e.Info) == 0 {
			return fmt.Errorf("transaction %s at position %d is missing locally", e.TransactionID, i)
		}
	}
	return nil
}

var (
	roleBurn   = int16(models.GetTokenRoleID(constants.TokenRole_Burn))
	roleCommit = int16(models.GetTokenRoleID(constants.TokenRole_Commit))
)

// CurrentOwner returns the DID owning tokenID at the chain tip, or "" if none.
// It reads the transaction info (quorum and fullnode store roles and token
// rows differently), newest entry first:
//   - burn/commit role or in CommittedTokens: no owner
//   - pledged by quorum q: q
//   - RBT/FT/SC: Owner, else Initiator
//   - NFT: Owner, else Initiator, only at deploy or when it is the sole asset
//     (otherwise the owner is not recorded)
//   - not named: genesis Owner/Initiator, or carried over from the previous entry
func CurrentOwner(tokenID string, chain []ChainEntry) (string, error) {
	for i := len(chain) - 1; i >= 0; i-- {
		e := chain[i]
		if e.Role == roleBurn || e.Role == roleCommit {
			return "", nil
		}
		var info models.TransactionInfo
		if err := json.Unmarshal(e.Info, &info); err != nil {
			return "", fmt.Errorf("decode transaction %s: %w", e.TransactionID, err)
		}
		owner := info.Owner
		if owner == "" {
			owner = info.Initiator
		}
		if ContainsToken(info.CommittedTokens, tokenID) {
			return "", nil
		}
		for _, q := range info.Quorums {
			if q != nil && ContainsToken(q.Tokens, tokenID) {
				return q.Did, nil
			}
		}
		if t := info.Tokens; t != nil {
			if ContainsToken(t.RBT, tokenID) || ContainsToken(t.FT, tokenID) || ContainsToken(t.SmartContract, tokenID) {
				return owner, nil
			}
			if ContainsToken(t.NFT, tokenID) {
				if i == 0 || (len(t.NFT) == 1 && len(t.RBT) == 0 && len(t.FT) == 0 && len(t.SmartContract) == 0) {
					return owner, nil
				}
				return "", nil
			}
		}
		if i == 0 {
			return owner, nil
		}
	}
	return "", nil
}

// ContainsToken reports whether list has an entry for tokenID.
func ContainsToken(list []*models.TokenInfo, tokenID string) bool {
	for _, t := range list {
		if t != nil && t.TokenID == tokenID {
			return true
		}
	}
	return false
}
