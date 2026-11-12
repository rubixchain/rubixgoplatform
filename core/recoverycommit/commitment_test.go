package recoverycommit

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/util"
)

func mustToken(t *testing.T, tokenID string, txIDs ...string) TokenCommitment {
	t.Helper()
	tc, err := BuildTokenCommitment(tokenID, txIDs)
	if err != nil {
		t.Fatalf("BuildTokenCommitment(%s): %v", tokenID, err)
	}
	return tc
}

func TestBuildTokenCommitment(t *testing.T) {
	tc := mustToken(t, "T1", "tx1", "tx2", "tx3", "tx4")
	want := util.HexToStr(ChainHash([]string{"tx1", "tx2", "tx3", "tx4"}))
	if tc.ChainRoot != want {
		t.Fatalf("chain root = %s, want %s", tc.ChainRoot, want)
	}

	for name, ids := range map[string][]string{"no transactions": nil, "empty tx ID": {"tx1", ""}} {
		if _, err := BuildTokenCommitment("T", ids); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	if _, err := BuildTokenCommitment("", []string{"tx1"}); err == nil {
		t.Fatal("empty token ID: expected error")
	}
}

func TestBuildQuorumCommitmentMatchesSpec(t *testing.T) {
	t1 := mustToken(t, "T1", "a1", "a2", "a3", "a4")
	t2 := mustToken(t, "T2", "b1", "b2", "b3")
	t3 := mustToken(t, "T3", "c1")

	tree, err := BuildQuorumCommitment("req-1", "user", "quorum", []TokenCommitment{t3, t1, t2})
	if err != nil {
		t.Fatal(err)
	}
	leaf := func(tc TokenCommitment) []byte { return TokenLeafHash(tc.TokenID, util.StrToHex(tc.ChainRoot)) }
	want := util.HexToStr(MerkleRoot([][]byte{leaf(t1), leaf(t2), leaf(t3)}))

	c := tree.Commitment
	if c.Root != want {
		t.Fatalf("root = %s, want %s", c.Root, want)
	}
	if c.RequestID != "req-1" || c.UserDID != "user" || c.QuorumDID != "quorum" || c.TokenCount != 3 {
		t.Fatalf("unexpected commitment fields: %+v", c)
	}
	if tree.Tokens[0].TokenID != "T1" || tree.Tokens[1].TokenID != "T2" || tree.Tokens[2].TokenID != "T3" {
		t.Fatalf("tokens not sorted: %+v", tree.Tokens)
	}
}

func TestBuildQuorumCommitmentDeterministicOrdering(t *testing.T) {
	a := mustToken(t, "10001_1", "x1", "x2")
	b := mustToken(t, "10001_10", "y1")
	c := mustToken(t, "10001_2", "z1", "z2", "z3")
	input := []TokenCommitment{c, a, b}

	first, err := BuildQuorumCommitment("r", "u", "q", input)
	if err != nil {
		t.Fatal(err)
	}
	if input[0].TokenID != "10001_2" {
		t.Fatal("input slice must not be reordered")
	}
	for _, perm := range [][]TokenCommitment{{a, b, c}, {b, c, a}, {c, b, a}} {
		got, err := BuildQuorumCommitment("r", "u", "q", perm)
		if err != nil {
			t.Fatal(err)
		}
		if got.Commitment.Root != first.Commitment.Root {
			t.Fatal("root must not depend on input order")
		}
	}
	// Bytewise order: "10001_1" < "10001_10" < "10001_2".
	if first.Tokens[0].TokenID != "10001_1" || first.Tokens[1].TokenID != "10001_10" || first.Tokens[2].TokenID != "10001_2" {
		t.Fatalf("unexpected order: %v, %v, %v", first.Tokens[0].TokenID, first.Tokens[1].TokenID, first.Tokens[2].TokenID)
	}
}

func TestBuildQuorumCommitmentSensitivity(t *testing.T) {
	base, _ := BuildQuorumCommitment("r", "u", "q", []TokenCommitment{mustToken(t, "T1", "a", "b"), mustToken(t, "T2", "c")})

	cases := map[string][]TokenCommitment{
		"extra transaction":  {mustToken(t, "T1", "a", "b", "x"), mustToken(t, "T2", "c")},
		"missing token":      {mustToken(t, "T1", "a", "b")},
		"extra token":        {mustToken(t, "T1", "a", "b"), mustToken(t, "T2", "c"), mustToken(t, "T3", "d")},
		"renamed token":      {mustToken(t, "T1", "a", "b"), mustToken(t, "T9", "c")},
		"reordered tx chain": {mustToken(t, "T1", "b", "a"), mustToken(t, "T2", "c")},
	}
	for name, tokens := range cases {
		got, err := BuildQuorumCommitment("r", "u", "q", tokens)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Commitment.Root == base.Commitment.Root {
			t.Fatalf("%s: root must change", name)
		}
	}
}

func TestBuildQuorumCommitmentEdgeCases(t *testing.T) {
	t.Run("no tokens", func(t *testing.T) {
		tree, err := BuildQuorumCommitment("r", "u", "q", nil)
		if err != nil {
			t.Fatal(err)
		}
		if tree.Commitment.TokenCount != 0 || tree.Commitment.Root != util.HexToStr(hash()) {
			t.Fatalf("unexpected empty commitment: %+v", tree.Commitment)
		}
	})
	t.Run("single token root is its token leaf", func(t *testing.T) {
		tc := mustToken(t, "T1", "a")
		tree, err := BuildQuorumCommitment("r", "u", "q", []TokenCommitment{tc})
		if err != nil {
			t.Fatal(err)
		}
		if tree.Commitment.Root != util.HexToStr(TokenLeafHash("T1", util.StrToHex(tc.ChainRoot))) {
			t.Fatal("single-token root mismatch")
		}
	})
	t.Run("duplicate token", func(t *testing.T) {
		if _, err := BuildQuorumCommitment("r", "u", "q", []TokenCommitment{mustToken(t, "T1", "a"), mustToken(t, "T1", "b")}); err == nil {
			t.Fatal("expected duplicate-token error")
		}
	})
	t.Run("malformed chain root", func(t *testing.T) {
		if _, err := BuildQuorumCommitment("r", "u", "q", []TokenCommitment{{TokenID: "T1", ChainRoot: "zz"}}); err == nil {
			t.Fatal("expected malformed-root error")
		}
	})
}

func TestCommitmentProofs(t *testing.T) {
	tokens := []TokenCommitment{
		mustToken(t, "T1", "a1", "a2", "a3", "a4"),
		mustToken(t, "T2", "b1", "b2", "b3"),
		mustToken(t, "T3", "c1"),
	}
	tree, err := BuildQuorumCommitment("r", "u", "q", tokens)
	if err != nil {
		t.Fatal(err)
	}
	root := tree.Commitment.Root

	for _, tc := range tree.Tokens {
		p, err := tree.TokenProof(tc.TokenID)
		if err != nil {
			t.Fatal(err)
		}
		if !VerifyTokenProof(root, tc.TokenID, tc.ChainRoot, p) {
			t.Fatalf("token proof for %s rejected", tc.TokenID)
		}
		if VerifyTokenProof(root, tc.TokenID, tree.Tokens[(p.Index+1)%len(tree.Tokens)].ChainRoot, p) {
			t.Fatalf("token proof for %s accepted with another token's chain root", tc.TokenID)
		}
	}
	if _, err := tree.TokenProof("missing"); err == nil {
		t.Fatal("expected error for unknown token")
	}
	if VerifyTokenProof(root, "T1", tree.Tokens[0].ChainRoot, nil) {
		t.Fatal("nil proof must be rejected")
	}
}
