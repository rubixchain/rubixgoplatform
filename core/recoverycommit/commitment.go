package recoverycommit

import (
	"fmt"
	"sort"

	"github.com/rubixchain/rubixgoplatform/util"
)

// Commitment is what a quorum publishes for one recovery request: the
// user-level Merkle root over every token it holds for UserDID.
type Commitment struct {
	RequestID  string `json:"request_id"`
	UserDID    string `json:"user_did"`
	QuorumDID  string `json:"quorum_did"`
	TokenCount int    `json:"token_count"`
	Root       string `json:"root"` // hex SHA3-256
}

// TokenCommitment is one token's layer-1 result.
type TokenCommitment struct {
	TokenID   string `json:"token_id"`
	ChainRoot string `json:"chain_root"` // hex SHA3-256 chain hash
}

// CommitmentTree is a Commitment plus everything needed to prove any token
// under it. Tokens is sorted by TokenID, which is the layer-2 leaf
// order.
type CommitmentTree struct {
	Commitment Commitment        `json:"commitment"`
	Tokens     []TokenCommitment `json:"tokens"`
}

// ExcludedToken records a candidate token that was left out of a commitment
// and why. It is diagnostic only and is not part of the committed root.
type ExcludedToken struct {
	TokenID string `json:"token_id"`
	Reason  string `json:"reason"`
}

// Proof is a Merkle audit path for the leaf at Index in a tree of Size leaves.
type Proof struct {
	Index int      `json:"index"`
	Size  int      `json:"size"`
	Path  []string `json:"path"` // hex hashes, leaf sibling first
}

// BuildTokenCommitment computes the layer-1 chain hash over txIDs, which must
// be the token's complete chain in position order.
func BuildTokenCommitment(tokenID string, txIDs []string) (TokenCommitment, error) {
	if tokenID == "" {
		return TokenCommitment{}, fmt.Errorf("BuildTokenCommitment: empty token ID")
	}
	if len(txIDs) == 0 {
		return TokenCommitment{}, fmt.Errorf("BuildTokenCommitment: token %s has no transactions", tokenID)
	}
	for i, id := range txIDs {
		if id == "" {
			return TokenCommitment{}, fmt.Errorf("BuildTokenCommitment: token %s has an empty transaction ID at position %d", tokenID, i)
		}
	}
	return TokenCommitment{
		TokenID:   tokenID,
		ChainRoot: util.HexToStr(ChainHash(txIDs)),
	}, nil
}

// BuildQuorumCommitment sorts tokens by token ID and computes the layer-2
// root over Hash(TokenID || ChainRoot). The input slice is not modified.
func BuildQuorumCommitment(requestID, userDID, quorumDID string, tokens []TokenCommitment) (*CommitmentTree, error) {
	if requestID == "" || userDID == "" || quorumDID == "" {
		return nil, fmt.Errorf("BuildQuorumCommitment: request ID, user DID and quorum DID are required")
	}
	sorted := append([]TokenCommitment(nil), tokens...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TokenID < sorted[j].TokenID })

	leaves, err := tokenLeaves(sorted)
	if err != nil {
		return nil, fmt.Errorf("BuildQuorumCommitment: %w", err)
	}
	return &CommitmentTree{
		Commitment: Commitment{
			RequestID:  requestID,
			UserDID:    userDID,
			QuorumDID:  quorumDID,
			TokenCount: len(sorted),
			Root:       util.HexToStr(MerkleRoot(leaves)),
		},
		Tokens: sorted,
	}, nil
}

// tokenLeaves validates sorted (unique, non-empty IDs, well-formed chain
// roots) and returns its layer-2 leaves.
func tokenLeaves(sorted []TokenCommitment) ([][]byte, error) {
	leaves := make([][]byte, len(sorted))
	for i, t := range sorted {
		if t.TokenID == "" {
			return nil, fmt.Errorf("empty token ID")
		}
		if i > 0 && sorted[i-1].TokenID == t.TokenID {
			return nil, fmt.Errorf("duplicate token %s", t.TokenID)
		}
		root := util.StrToHex(t.ChainRoot)
		if len(root) != HashSize {
			return nil, fmt.Errorf("token %s has malformed chain root %q", t.TokenID, t.ChainRoot)
		}
		leaves[i] = TokenLeafHash(t.TokenID, root)
	}
	return leaves, nil
}

func (t *CommitmentTree) tokenIndex(tokenID string) (int, error) {
	i := sort.Search(len(t.Tokens), func(i int) bool { return t.Tokens[i].TokenID >= tokenID })
	if i == len(t.Tokens) || t.Tokens[i].TokenID != tokenID {
		return 0, fmt.Errorf("token %s is not in the commitment", tokenID)
	}
	return i, nil
}

// TokenProof proves that tokenID, with its ChainRoot, is a leaf under
// Commitment.Root.
func (t *CommitmentTree) TokenProof(tokenID string) (*Proof, error) {
	i, err := t.tokenIndex(tokenID)
	if err != nil {
		return nil, err
	}
	leaves, err := tokenLeaves(t.Tokens)
	if err != nil {
		return nil, err
	}
	return newProof(leaves, i)
}

func newProof(leaves [][]byte, index int) (*Proof, error) {
	path, err := MerkleProof(leaves, index)
	if err != nil {
		return nil, err
	}
	p := &Proof{Index: index, Size: len(leaves), Path: make([]string, len(path))}
	for i, h := range path {
		p.Path[i] = util.HexToStr(h)
	}
	return p, nil
}

// VerifyTokenProof checks that (tokenID, chainRoot) is the leaf p proves under
// the user-level root.
func VerifyTokenProof(root, tokenID, chainRoot string, p *Proof) bool {
	cr := util.StrToHex(chainRoot)
	if len(cr) != HashSize {
		return false
	}
	return verifyProof(TokenLeafHash(tokenID, cr), root, p)
}

func verifyProof(leaf []byte, root string, p *Proof) bool {
	if p == nil {
		return false
	}
	r := util.StrToHex(root)
	if len(r) != HashSize {
		return false
	}
	path := make([][]byte, len(p.Path))
	for i, h := range p.Path {
		if path[i] = util.StrToHex(h); len(path[i]) != HashSize {
			return false
		}
	}
	return VerifyMerkleProof(leaf, p.Index, p.Size, path, r)
}

// CompareCommitments lists every field in which submitted differs from
// expected. An empty result means the commitments match.
func CompareCommitments(expected, submitted Commitment) []string {
	var diffs []string
	check := func(field, want, got string) {
		if want != got {
			diffs = append(diffs, fmt.Sprintf("%s: expected %q, got %q", field, want, got))
		}
	}
	check("request_id", expected.RequestID, submitted.RequestID)
	check("user_did", expected.UserDID, submitted.UserDID)
	check("quorum_did", expected.QuorumDID, submitted.QuorumDID)
	if expected.TokenCount != submitted.TokenCount {
		diffs = append(diffs, fmt.Sprintf("token_count: expected %d, got %d", expected.TokenCount, submitted.TokenCount))
	}
	check("root", expected.Root, submitted.Root)
	return diffs
}
