package core

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/types"
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

	// x9 moves FT F9 to the user, pledged by q2. Recovery covers RBTs only.
	x9 := rcRBT("F9", "alice", user, "q2")
	x9.Tokens.FT, x9.Tokens.RBT = x9.Tokens.RBT, nil

	// Tokens whose fullnode tip names the user as owner.
	chains := map[string][]types.RecoveryChainEntry{
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
		"T8": func() []types.RecoveryChainEntry {
			c := rcChain(t,
				rcTx{"m8", mint, rcRBT("T8", "alice", "alice")},
				rcTx{"a8", xfer, rcRBT("T8", "alice", "carol", "q2")},
				rcTx{"b8", xfer, rcRBT("T8", "carol", user, "q1")},
			)
			return append(c[:1], c[2:]...)
		}(),
		"F9": rcChain(t,
			rcTx{"m9", mint, rcRBT("F9", "alice", "alice")},
			rcTx{"x9", xfer, x9},
		),
	}

	quorumCommit := func(quorum string, pledged map[string][]string) types.RecoveryCommitment {
		tokens, _ := selectQuorumPledgedTokens(pledged, chains)
		tree, err := recoverycommit.BuildQuorumCommitment(req, user, quorum, tokens)
		if err != nil {
			t.Fatal(err)
		}
		return tree.Commitment
	}
	honestQ1 := quorumCommit("q1", map[string][]string{"T1": {"x1"}, "T3": {"a3", "c3"}})

	report, err := verifyQuorumCommitments(req, user, chains, []types.RecoveryCommitment{honestQ1})
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
	empty := quorumCommit("q2", nil)
	altered := honestQ1
	altered.Root = missingToken.Root
	for name, sub := range map[string]types.RecoveryCommitment{
		"missing token": missingToken, "extra token": extraToken, "altered root": altered,
		"empty commitment from a quorum holding nothing": empty,
	} {
		report, err := verifyQuorumCommitments(req, user, chains, []types.RecoveryCommitment{sub})
		if err != nil {
			t.Fatal(err)
		}
		if report.Checks[0].Match {
			t.Errorf("%s: commitment must not match", name)
		}
	}
}

func TestSelectUnpledgedTokens(t *testing.T) {
	info := rcRBT("T", "alice", "user")
	info.Quorums = []*models.QuorumInfo{{Did: "q", Tokens: []*models.TokenInfo{{TokenID: "P"}}}}
	transfer := rcChain(t, rcTx{"x", constants.TokenRole_Transfer, info})
	pledge := rcChain(t, rcTx{"x", constants.TokenRole_Pledge, info})
	release := rcRBT("sibling", "user", "other")
	collateral := rcChain(t, rcTx{"x", constants.TokenRole_Pledge, info}, rcTx{"y", constants.TokenRole_Unpledge, release})
	for _, tc := range []struct {
		name                     string
		collateral               []types.RecoveryChainEntry
		wantTokens, wantExcluded int
	}{
		{"released collateral", collateral, 1, 0},
		{"live pledge", pledge, 0, 0},
		{"unknown pledge state", nil, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chains := map[string][]types.RecoveryChainEntry{"T": transfer}
			if tc.collateral != nil {
				chains["P"] = tc.collateral
			}
			tokens, excluded := selectUnpledgedTokens("user", chains)
			if len(tokens) != tc.wantTokens || len(excluded) != tc.wantExcluded {
				t.Fatalf("tokens=%v excluded=%v", tokens, excluded)
			}
		})
	}
	// A quorum can recover its own collateral after its pledge is released.
	got, excluded := selectUnpledgedTokens("q", map[string][]types.RecoveryChainEntry{"P": collateral})
	if len(got) != 1 || got[0].TokenID != "P" || len(excluded) != 0 {
		t.Fatalf("released quorum collateral: tokens=%v excluded=%v", got, excluded)
	}
}

func TestRecoveryCommitmentsOnlyIncludeRBTs(t *testing.T) {
	info := rcRBT("R", "alice", "user")
	info.Tokens.FT = []*models.TokenInfo{{TokenID: "F"}}
	info.Tokens.NFT = []*models.TokenInfo{{TokenID: "N"}}
	info.Tokens.SmartContract = []*models.TokenInfo{{TokenID: "S"}}
	chains := make(map[string][]types.RecoveryChainEntry)
	pledged := make(map[string][]string)
	for _, tokenID := range []string{"R", "F", "N", "S"} {
		chains[tokenID] = rcChain(t, rcTx{"x", constants.TokenRole_Transfer, info})
		pledged[tokenID] = []string{"x"}
	}
	for name, selectTokens := range map[string]func() []types.RecoveryTokenCommitment{
		"quorum": func() []types.RecoveryTokenCommitment {
			tokens, _ := selectQuorumPledgedTokens(pledged, chains)
			return tokens
		},
		"fullnode": func() []types.RecoveryTokenCommitment {
			tokens, _ := selectUnpledgedTokens("user", chains)
			return tokens
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := selectTokens()
			if len(got) != 1 || got[0].TokenID != "R" {
				t.Fatalf("expected only RBT, got %v", got)
			}
		})
	}
	info.Quorums = []*models.QuorumInfo{{Did: "q", Tokens: []*models.TokenInfo{{TokenID: "P"}}}}
	for tokenID := range chains {
		chains[tokenID] = rcChain(t, rcTx{"x", constants.TokenRole_Transfer, info})
	}
	expected, _ := expectedPledgedTokens("user", chains)
	if len(expected["q"]) != 1 || expected["q"][0].TokenID != "R" {
		t.Fatalf("quorum reproduction included other assets: %v", expected)
	}
}

func TestCollectRecoveryCommitments(t *testing.T) {
	tx := func(initiator, data string) types.RecoveryTx {
		return types.RecoveryTx{Info: models.TransactionInfo{Initiator: initiator}, Data: data}
	}
	commitment := func(sender, committerDID string) string {
		return `{"type":"commitment","body":{"request_id":"r","committer_did":"` + committerDID + `","committer_type":"quorum"},"sender_did":"` + sender + `"}`
	}
	chain := []types.RecoveryTx{
		tx("user", ""),                   // deploy
		tx("q1", commitment("q1", "q1")), // kept
		tx("q2", commitment("q2", "q1")), // commits in q1's name
		tx("q3", commitment("q1", "q1")), // sender is not the initiator
		tx("x", `{"type":"challenge","body":{},"sender_did":"x"}`),
		tx("q4", commitment("q4", "q4")), // kept
	}
	got := collectRecoveryCommitments(chain, "user")
	if len(got) != 2 || got[0].CommitterDID != "q1" || got[1].CommitterDID != "q4" {
		t.Fatalf("got %+v, want commitments from q1 and q4", got)
	}
}
