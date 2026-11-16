package core

import (
	"encoding/json"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// rcTx is one step of a test token's history.
type rcTx struct {
	id   string
	role string
	info models.TransactionInfo
}

func rcRBT(tokenID, initiator, owner string, quorums ...string) models.TransactionInfo {
	info := models.TransactionInfo{
		Initiator: initiator,
		Owner:     owner,
		Tokens:    &models.TransactionTokens{RBT: []*models.TokenInfo{{TokenID: tokenID}}},
	}
	for _, q := range quorums {
		info.Quorums = append(info.Quorums, &models.QuorumInfo{Did: q, Tokens: []*models.TokenInfo{{TokenID: q + "-pledge"}}})
	}
	return info
}

func rcChain(t *testing.T, txs ...rcTx) []recoverycommit.ChainEntry {
	t.Helper()
	chain := make([]recoverycommit.ChainEntry, len(txs))
	for i, tx := range txs {
		b, err := json.Marshal(tx.info)
		if err != nil {
			t.Fatal(err)
		}
		chain[i] = recoverycommit.ChainEntry{
			TransactionID: tx.id,
			Position:      int64(i),
			Role:          int16(models.GetTokenRoleID(tx.role)),
			Info:          b,
		}
		if i > 0 {
			chain[i].PreviousTransactionID = txs[i-1].id
		}
	}
	return chain
}

func rcIDs(tokens []recoverycommit.TokenCommitment) map[string]string {
	out := make(map[string]string, len(tokens))
	for _, tc := range tokens {
		out[tc.TokenID] = tc.ChainRoot
	}
	return out
}

func TestSelectQuorumPledgedTokens(t *testing.T) {
	const user = "user"
	mint, xfer := constants.TokenRole_Mint, constants.TokenRole_Transfer

	chains := map[string][]recoverycommit.ChainEntry{
		// Pledged transaction is the local tip.
		"T1": rcChain(t,
			rcTx{"m1", mint, rcRBT("T1", "alice", "alice")},
			rcTx{"x1", xfer, rcRBT("T1", "alice", user, "q1")},
		),
		// The chain moved on after the pledge: the row is stale.
		"T2": rcChain(t,
			rcTx{"m2", mint, rcRBT("T2", "alice", "alice")},
			rcTx{"x2", xfer, rcRBT("T2", "alice", user, "q1")},
			rcTx{"y2", xfer, rcRBT("T2", user, "bob", "q1")},
		),
		// Re-received: a stale and a newer live pledge; the newer one is the tip.
		"T3": rcChain(t,
			rcTx{"m3", mint, rcRBT("T3", "alice", "alice")},
			rcTx{"a3", xfer, rcRBT("T3", "alice", user, "q1")},
			rcTx{"b3", xfer, rcRBT("T3", user, "carol", "q2")},
			rcTx{"c3", xfer, rcRBT("T3", "carol", user, "q1")},
		),
		// Gap before the pledged transaction.
		"T5": func() []recoverycommit.ChainEntry {
			c := rcChain(t,
				rcTx{"m5", mint, rcRBT("T5", "alice", "alice")},
				rcTx{"a5", xfer, rcRBT("T5", "alice", "carol", "q2")},
				rcTx{"b5", xfer, rcRBT("T5", "carol", user, "q1")},
			)
			return append(c[:1], c[2:]...)
		}(),
	}
	pledged := map[string][]string{
		"T1": {"x1"},
		"T2": {"x2"},
		"T3": {"a3", "c3"},
		"T4": {"x4"}, // no local chain
		"T5": {"b5"},
	}

	tokens, excluded := selectQuorumPledgedTokens(pledged, chains)
	got := rcIDs(tokens)
	if len(got) != 2 {
		t.Fatalf("selected tokens = %v, want T1 and T3", got)
	}
	for id, txIDs := range map[string][]string{
		"T1": {"m1", "x1"},
		"T3": {"m3", "a3", "b3", "c3"},
	} {
		want, _ := recoverycommit.BuildTokenCommitment(id, txIDs)
		if got[id] != want.ChainRoot {
			t.Errorf("%s chain root must cover %v", id, txIDs)
		}
	}
	if len(excluded) != 3 || excluded[0].TokenID != "T2" || excluded[1].TokenID != "T4" || excluded[2].TokenID != "T5" {
		t.Fatalf("excluded = %+v, want T2, T4 and T5", excluded)
	}
}
