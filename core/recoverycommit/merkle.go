// Package recoverycommit builds the account-recovery commitment, shared by
// quorum and fullnode (pure code, no I/O):
//   - layer 1: per token, a hash over its transaction IDs, genesis to latest
//   - layer 2: per quorum, a Merkle root over H(TokenID || ChainRoot), sorted by token ID
package recoverycommit

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/util"
)

// Domain-separation prefixes for the layer-2 Merkle tree, as in RFC 6962
// (Certificate Transparency): a token leaf and an internal node can never be
// reinterpreted as one another.
const (
	prefixLeaf byte = 0x00
	prefixNode byte = 0x01
)

// HashSize is the length in bytes of every hash and root (SHA3-256).
const HashSize = 32

func hash(parts ...[]byte) []byte {
	return util.CalculateHash(bytes.Join(parts, nil), constants.HashAlgorithm_SHA3_256)
}

// lengthPrefixed encodes s as a 4-byte big-endian length followed by its bytes,
// so concatenated variable-length fields cannot be split two ways.
func lengthPrefixed(s string) []byte {
	out := make([]byte, 4+len(s))
	binary.BigEndian.PutUint32(out, uint32(len(s)))
	copy(out[4:], s)
	return out
}

// ChainHash is the layer-1 hash of a tokenchain:
// H(len(tx0) || tx0 || len(tx1) || tx1 || ... || len(txN) || txN), with the
// transaction IDs in chain order from genesis to the latest transaction.
func ChainHash(txIDs []string) []byte {
	parts := make([][]byte, len(txIDs))
	for i, id := range txIDs {
		parts[i] = lengthPrefixed(id)
	}
	return hash(parts...)
}

// TokenLeafHash is the layer-2 leaf for one token:
// H(0x00 || len(tokenID) || tokenID || chainRoot). chainRoot is fixed-size, so
// only the token ID needs a length prefix.
func TokenLeafHash(tokenID string, chainRoot []byte) []byte {
	return hash([]byte{prefixLeaf}, lengthPrefixed(tokenID), chainRoot)
}

func nodeHash(left, right []byte) []byte {
	return hash([]byte{prefixNode}, left, right)
}

// splitPoint returns the largest power of two strictly less than n (n >= 2).
func splitPoint(n int) int {
	k := 1
	for k<<1 < n {
		k <<= 1
	}
	return k
}

// MerkleRoot is the RFC 6962 tree hash over leaves: H() for none, the leaf
// itself for one, else node(root(first k), root(rest)) with k the largest
// power of 2 < n. Odd counts never duplicate the last leaf.
func MerkleRoot(leaves [][]byte) []byte {
	switch len(leaves) {
	case 0:
		// Only occurs at layer 2, when a quorum holds no tokens for the user.
		return hash()
	case 1:
		return leaves[0]
	}
	k := splitPoint(len(leaves))
	return nodeHash(MerkleRoot(leaves[:k]), MerkleRoot(leaves[k:]))
}

// MerkleProof returns the RFC 6962 audit path for the leaf at index, ordered
// from the leaf's sibling up to the child of the root.
func MerkleProof(leaves [][]byte, index int) ([][]byte, error) {
	if index < 0 || index >= len(leaves) {
		return nil, fmt.Errorf("MerkleProof: index %d out of range for %d leaves", index, len(leaves))
	}
	return auditPath(leaves, index), nil
}

func auditPath(leaves [][]byte, index int) [][]byte {
	if len(leaves) <= 1 {
		return nil
	}
	k := splitPoint(len(leaves))
	if index < k {
		return append(auditPath(leaves[:k], index), MerkleRoot(leaves[k:]))
	}
	return append(auditPath(leaves[k:], index-k), MerkleRoot(leaves[:k]))
}

// VerifyMerkleProof checks that leaf sits at index in a tree of size leaves
// whose root is root (RFC 9162 section 2.1.3.2).
func VerifyMerkleProof(leaf []byte, index, size int, proof [][]byte, root []byte) bool {
	if index < 0 || index >= size {
		return false
	}
	fn, sn := index, size-1
	r := leaf
	for _, p := range proof {
		if sn == 0 {
			return false
		}
		if fn&1 == 1 || fn == sn {
			r = nodeHash(p, r)
			for fn&1 == 0 && fn != 0 {
				fn >>= 1
				sn >>= 1
			}
		} else {
			r = nodeHash(r, p)
		}
		fn >>= 1
		sn >>= 1
	}
	return sn == 0 && bytes.Equal(r, root)
}
