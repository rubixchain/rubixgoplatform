package core

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// The fullnode's reproduction must equal what the quorum commits from the
// same chains, and must flag a quorum that commits a different token set.
func TestVerifyQuorumCommitments(t *testing.T) {
	const req, user = "req", "user"
	mint, xfer := constants.TokenRole_Mint, constants.TokenRole_Transfer

	// x6 moves T6 and T7 together, pledged by q2; T7 later leaves the user.
	x6 := rcRBT("T6", "alice", user, "q2")
	x6.Tokens.RBT = append(x6.Tokens.RBT, &models.TokenInfo{TokenID: "T7"})

	// Tokens whose fullnode tip names the user as owner.
	chains := map[string][]recoverycommit.ChainEntry{
		"T1": rcChain(t,
			rcTx{"m1", mint, rcRBT("T1", "alice", "alice")},
			rcTx{"x1", xfer, rcRBT("T1", "alice", user, "q1")},
		),
		// Re-received: only the latest pledging quorum (q1) commits it.
		"T3": rcChain(t,
			rcTx{"m3", mint, rcRBT("T3", "alice", "alice")},
			rcTx{"a3", xfer, rcRBT("T3", "alice", user, "q1")},
			rcTx{"b3", xfer, rcRBT("T3", user, "carol", "q2")},
			rcTx{"c3", xfer, rcRBT("T3", "carol", user, "q1")},
		),
		// T7 moved on, so q2 unpledged x6 and T6 is in no quorum's set.
		"T6": rcChain(t,
			rcTx{"m6", mint, rcRBT("T6", "alice", "alice")},
			rcTx{"x6", xfer, x6},
		),
		// Gap in the fullnode chain.
		"T8": func() []recoverycommit.ChainEntry {
			c := rcChain(t,
				rcTx{"m8", mint, rcRBT("T8", "alice", "alice")},
				rcTx{"a8", xfer, rcRBT("T8", "alice", "carol", "q2")},
				rcTx{"b8", xfer, rcRBT("T8", "carol", user, "q1")},
			)
			return append(c[:1], c[2:]...)
		}(),
	}

	quorumCommit := func(quorum string, pledged map[string][]string) recoverycommit.Commitment {
		tokens, _ := selectQuorumPledgedTokens(pledged, chains)
		tree, err := recoverycommit.BuildQuorumCommitment(req, user, quorum, tokens)
		if err != nil {
			t.Fatal(err)
		}
		return tree.Commitment
	}
	honestQ1 := quorumCommit("q1", map[string][]string{"T1": {"x1"}, "T3": {"a3", "c3"}})

	report, err := verifyQuorumCommitments(req, user, chains, []recoverycommit.Commitment{honestQ1})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Checks) != 1 || !report.Checks[0].Match || len(report.MissingQuorums) != 0 {
		t.Fatalf("honest q1 must match with no missing quorums: %+v", report)
	}
	if ex := report.Excluded; len(ex) != 2 || ex[0].TokenID != "T6" || ex[1].TokenID != "T8" {
		t.Fatalf("excluded = %+v, want T6 and T8", ex)
	}

	missingToken := quorumCommit("q1", map[string][]string{"T1": {"x1"}})
	extraToken := quorumCommit("q2", map[string][]string{"T6": {"x6"}})
	altered := honestQ1
	altered.Root = missingToken.Root
	for name, sub := range map[string]recoverycommit.Commitment{
		"missing token": missingToken, "extra token": extraToken, "altered root": altered,
	} {
		report, err := verifyQuorumCommitments(req, user, chains, []recoverycommit.Commitment{sub})
		if err != nil {
			t.Fatal(err)
		}
		if report.Checks[0].Match {
			t.Errorf("%s: commitment must not match", name)
		}
	}
}
