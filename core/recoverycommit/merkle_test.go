package recoverycommit

import (
	"bytes"
	"fmt"
	"testing"
)

func testLeaves(n int) [][]byte {
	leaves := make([][]byte, n)
	for i := range leaves {
		leaves[i] = TokenLeafHash(fmt.Sprintf("T%d", i), ChainHash([]string{fmt.Sprintf("tx%d", i)}))
	}
	return leaves
}

func TestMerkleRootShape(t *testing.T) {
	l := testLeaves(5)

	t.Run("empty tree is H()", func(t *testing.T) {
		if !bytes.Equal(MerkleRoot(nil), hash()) {
			t.Fatal("empty root mismatch")
		}
	})
	t.Run("single leaf is the leaf hash", func(t *testing.T) {
		if !bytes.Equal(MerkleRoot(l[:1]), l[0]) {
			t.Fatal("single-leaf root must equal the leaf")
		}
	})
	t.Run("two leaves", func(t *testing.T) {
		if !bytes.Equal(MerkleRoot(l[:2]), nodeHash(l[0], l[1])) {
			t.Fatal("two-leaf root mismatch")
		}
	})
	t.Run("three leaves split 2+1", func(t *testing.T) {
		want := nodeHash(nodeHash(l[0], l[1]), l[2])
		if !bytes.Equal(MerkleRoot(l[:3]), want) {
			t.Fatal("three-leaf root mismatch")
		}
	})
	t.Run("five leaves split 4+1", func(t *testing.T) {
		want := nodeHash(nodeHash(nodeHash(l[0], l[1]), nodeHash(l[2], l[3])), l[4])
		if !bytes.Equal(MerkleRoot(l), want) {
			t.Fatal("five-leaf root mismatch")
		}
	})
}

func TestChainHash(t *testing.T) {
	if bytes.Equal(ChainHash([]string{"a", "b"}), ChainHash([]string{"b", "a"})) {
		t.Fatal("reordering transactions must change the chain hash")
	}
	if bytes.Equal(ChainHash([]string{"ab", "c"}), ChainHash([]string{"a", "bc"})) {
		t.Fatal("transaction ID boundaries must be unambiguous")
	}
}

func TestDomainSeparation(t *testing.T) {
	a, b := testLeaves(2)[0], testLeaves(2)[1]
	// An internal node over (a, b) must not equal a token leaf built from the
	// same bytes.
	if bytes.Equal(nodeHash(a, b), TokenLeafHash(string(a), b)) {
		t.Fatal("internal node and token leaf must differ")
	}
	r := ChainHash([]string{"x"})
	if bytes.Equal(TokenLeafHash("ab", r), TokenLeafHash("a", append([]byte("b"), r[:HashSize-1]...))) {
		t.Fatal("token ID / chain root boundary must be unambiguous")
	}
}

func TestMerkleProofAllSizesAndIndices(t *testing.T) {
	for n := 1; n <= 17; n++ {
		leaves := testLeaves(n)
		root := MerkleRoot(leaves)
		for i := 0; i < n; i++ {
			path, err := MerkleProof(leaves, i)
			if err != nil {
				t.Fatalf("n=%d i=%d: %v", n, i, err)
			}
			if !VerifyMerkleProof(leaves[i], i, n, path, root) {
				t.Fatalf("n=%d i=%d: valid proof rejected", n, i)
			}
			if VerifyMerkleProof(TokenLeafHash("other", ChainHash([]string{"x"})), i, n, path, root) {
				t.Fatalf("n=%d i=%d: wrong leaf accepted", n, i)
			}
			if n > 1 && VerifyMerkleProof(leaves[i], (i+1)%n, n, path, root) {
				t.Fatalf("n=%d i=%d: wrong index accepted", n, i)
			}
		}
	}
}

func TestMerkleProofOutOfRange(t *testing.T) {
	if _, err := MerkleProof(testLeaves(3), 3); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
	if _, err := MerkleProof(nil, 0); err == nil {
		t.Fatal("expected error for empty tree")
	}
}
