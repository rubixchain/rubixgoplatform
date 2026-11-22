package core

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func rvChain(ids ...string) []types.TransactionWithRole {
	txs := make([]types.TransactionWithRole, len(ids))
	for i, id := range ids {
		txs[i] = types.TransactionWithRole{Tx: models.Transactions{ID: id}}
	}
	return txs
}

func TestVerifyRecoveryDeliveries(t *testing.T) {
	const req, user, q1, q2, fn = "req", "user", "q1", "q2", "fn"
	honest := map[string][]types.TransactionWithRole{
		"t1": rvChain("a", "b"),
		"t2": rvChain("c"),
	}
	t1, err := recoverycommit.BuildTokenCommitment("t1", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	t2, err := recoverycommit.BuildTokenCommitment("t2", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	tokens := []types.RecoveryTokenCommitment{t1, t2}
	quorumTree, err := recoverycommit.BuildQuorumCommitment(req, user, q1, tokens)
	if err != nil {
		t.Fatal(err)
	}
	pub := quorumTree.Commitment
	unpledgedTree, err := recoverycommit.BuildUnpledgedCommitment(req, user, fn, tokens)
	if err != nil {
		t.Fatal(err)
	}
	emptyTree, err := recoverycommit.BuildUnpledgedCommitment(req, user, fn, nil)
	if err != nil {
		t.Fatal(err)
	}

	otherReq := pub
	otherReq.RequestID = "other"

	cases := []struct {
		name      string
		published types.RecoveryCommitment
		unpledged bool // published as an unpledged message
		delivered map[string][]types.TransactionWithRole
		match     bool
	}{
		{"matching", pub, false, honest, true},
		{"missing token", pub, false, map[string][]types.TransactionWithRole{"t1": honest["t1"]}, false},
		{"extra token", pub, false, map[string][]types.TransactionWithRole{"t1": honest["t1"], "t2": honest["t2"], "t3": rvChain("d")}, false},
		{"altered transaction", pub, false, map[string][]types.TransactionWithRole{"t1": rvChain("a", "x"), "t2": honest["t2"]}, false},
		{"commitment for another request", otherReq, false, honest, false},
		{"no delivery", pub, false, nil, false},
		{"fullnode matching", unpledgedTree.Commitment, true, honest, true},
		{"fullnode no delivery", unpledgedTree.Commitment, true, nil, false},
		{"fullnode with no tokens and no delivery", emptyTree.Commitment, true, nil, true},
		{"quorum commitment published as unpledged", pub, true, honest, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deliveries := map[string]map[string][]types.TransactionWithRole{}
			if tc.delivered != nil {
				deliveries[tc.published.CommitterDID] = tc.delivered
			}
			var quorum, unpledged []types.RecoveryCommitment
			if tc.unpledged {
				unpledged = []types.RecoveryCommitment{tc.published}
			} else {
				quorum = []types.RecoveryCommitment{tc.published}
			}
			checks := verifyRecoveryDeliveries(req, user, quorum, unpledged, deliveries)
			if len(checks) != 1 {
				t.Fatalf("got %d checks, want 1", len(checks))
			}
			if checks[0].Match != tc.match {
				t.Fatalf("match = %v, want %v (mismatches %v)", checks[0].Match, tc.match, checks[0].Mismatches)
			}
			if !tc.match && len(checks[0].Mismatches) == 0 {
				t.Fatal("a failed check must say why")
			}
		})
	}

	// A delivery from one committer is not used for another's commitment.
	other := pub
	other.CommitterDID = q2
	checks := verifyRecoveryDeliveries(req, user, []types.RecoveryCommitment{other}, nil, map[string]map[string][]types.TransactionWithRole{q1: honest})
	if checks[0].Match {
		t.Fatal("q2's commitment matched q1's delivery")
	}
}
