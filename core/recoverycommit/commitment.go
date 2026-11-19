package recoverycommit

import (
	"fmt"
	"sort"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
)

// BuildTokenCommitment computes the layer-1 chain hash over txIDs, which must
// be the token's complete chain in position order.
func BuildTokenCommitment(tokenID string, txIDs []string) (types.RecoveryTokenCommitment, error) {
	if tokenID == "" {
		return types.RecoveryTokenCommitment{}, fmt.Errorf("BuildTokenCommitment: empty token ID")
	}
	if len(txIDs) == 0 {
		return types.RecoveryTokenCommitment{}, fmt.Errorf("BuildTokenCommitment: token %s has no transactions", tokenID)
	}
	for i, id := range txIDs {
		if id == "" {
			return types.RecoveryTokenCommitment{}, fmt.Errorf("BuildTokenCommitment: token %s has an empty transaction ID at position %d", tokenID, i)
		}
	}
	return types.RecoveryTokenCommitment{
		TokenID:   tokenID,
		ChainRoot: util.HexToStr(ChainHash(txIDs)),
	}, nil
}

// BuildQuorumCommitment sorts tokens by token ID and computes the layer-2
// root over Hash(TokenID || ChainRoot). The input slice is not modified.
func BuildQuorumCommitment(requestID, userDID, quorumDID string, tokens []types.RecoveryTokenCommitment) (*types.RecoveryCommitmentTree, error) {
	if requestID == "" || userDID == "" || quorumDID == "" {
		return nil, fmt.Errorf("BuildQuorumCommitment: request ID, user DID and quorum DID are required")
	}
	tree, err := buildCommitment(requestID, userDID, quorumDID, constants.RecoveryCommitterType_Quorum, tokens)
	if err != nil {
		return nil, fmt.Errorf("BuildQuorumCommitment: %w", err)
	}
	return tree, nil
}

// BuildUnpledgedCommitment builds the fullnode commitment for unpledged tokens.
// Token selection and pledge-state checks belong to the caller.
func BuildUnpledgedCommitment(requestID, userDID, fullnodeDID string, tokens []types.RecoveryTokenCommitment) (*types.RecoveryCommitmentTree, error) {
	if requestID == "" || userDID == "" || fullnodeDID == "" {
		return nil, fmt.Errorf("BuildUnpledgedCommitment: request ID, user DID and fullnode DID are required")
	}
	tree, err := buildCommitment(requestID, userDID, fullnodeDID, constants.RecoveryCommitterType_Fullnode, tokens)
	if err != nil {
		return nil, fmt.Errorf("BuildUnpledgedCommitment: %w", err)
	}
	return tree, nil
}

func buildCommitment(requestID, userDID, committerDID, committerType string, tokens []types.RecoveryTokenCommitment) (*types.RecoveryCommitmentTree, error) {
	sorted := append([]types.RecoveryTokenCommitment(nil), tokens...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TokenID < sorted[j].TokenID })
	leaves, err := tokenLeaves(sorted)
	if err != nil {
		return nil, err
	}
	return &types.RecoveryCommitmentTree{
		Commitment: types.RecoveryCommitment{
			RequestID:     requestID,
			UserDID:       userDID,
			CommitterDID:  committerDID,
			CommitterType: committerType,
			TokenCount:    len(sorted),
			Root:          util.HexToStr(MerkleRoot(leaves)),
		},
		Tokens: sorted,
	}, nil
}

// tokenLeaves validates sorted (unique, non-empty IDs, well-formed chain
// roots) and returns its layer-2 leaves.
func tokenLeaves(sorted []types.RecoveryTokenCommitment) ([][]byte, error) {
	leaves := make([][]byte, len(sorted))
	for i, t := range sorted {
		if t.TokenID == "" {
			return nil, fmt.Errorf("empty token ID")
		}
		if i > 0 && sorted[i-1].TokenID == t.TokenID {
			return nil, fmt.Errorf("duplicate token %s", t.TokenID)
		}
		root := util.StrToHex(t.ChainRoot)
		if len(root) != constants.RecoveryCommit_HashSize {
			return nil, fmt.Errorf("token %s has malformed chain root %q", t.TokenID, t.ChainRoot)
		}
		leaves[i] = TokenLeafHash(t.TokenID, root)
	}
	return leaves, nil
}

func tokenIndex(t *types.RecoveryCommitmentTree, tokenID string) (int, error) {
	i := sort.Search(len(t.Tokens), func(i int) bool { return t.Tokens[i].TokenID >= tokenID })
	if i == len(t.Tokens) || t.Tokens[i].TokenID != tokenID {
		return 0, fmt.Errorf("token %s is not in the commitment", tokenID)
	}
	return i, nil
}

// TokenProof proves that tokenID, with its ChainRoot, is a leaf under
// Commitment.Root.
func TokenProof(t *types.RecoveryCommitmentTree, tokenID string) (*types.RecoveryProof, error) {
	i, err := tokenIndex(t, tokenID)
	if err != nil {
		return nil, err
	}
	leaves, err := tokenLeaves(t.Tokens)
	if err != nil {
		return nil, err
	}
	return newProof(leaves, i)
}

func newProof(leaves [][]byte, index int) (*types.RecoveryProof, error) {
	path, err := MerkleProof(leaves, index)
	if err != nil {
		return nil, err
	}
	p := &types.RecoveryProof{Index: index, Size: len(leaves), Path: make([]string, len(path))}
	for i, h := range path {
		p.Path[i] = util.HexToStr(h)
	}
	return p, nil
}

// VerifyTokenProof checks that (tokenID, chainRoot) is the leaf p proves under
// the user-level root.
func VerifyTokenProof(root, tokenID, chainRoot string, p *types.RecoveryProof) bool {
	cr := util.StrToHex(chainRoot)
	if len(cr) != constants.RecoveryCommit_HashSize {
		return false
	}
	return verifyProof(TokenLeafHash(tokenID, cr), root, p)
}

func verifyProof(leaf []byte, root string, p *types.RecoveryProof) bool {
	if p == nil {
		return false
	}
	r := util.StrToHex(root)
	if len(r) != constants.RecoveryCommit_HashSize {
		return false
	}
	path := make([][]byte, len(p.Path))
	for i, h := range p.Path {
		if path[i] = util.StrToHex(h); len(path[i]) != constants.RecoveryCommit_HashSize {
			return false
		}
	}
	return VerifyMerkleProof(leaf, p.Index, p.Size, path, r)
}

// CompareCommitments lists every field in which submitted differs from
// expected. An empty result means the commitments match.
func CompareCommitments(expected, submitted types.RecoveryCommitment) []string {
	var diffs []string
	check := func(field, want, got string) {
		if want != got {
			diffs = append(diffs, fmt.Sprintf("%s: expected %q, got %q", field, want, got))
		}
	}
	check("request_id", expected.RequestID, submitted.RequestID)
	check("user_did", expected.UserDID, submitted.UserDID)
	check("committer_did", expected.CommitterDID, submitted.CommitterDID)
	check("committer_type", expected.CommitterType, submitted.CommitterType)
	if expected.TokenCount != submitted.TokenCount {
		diffs = append(diffs, fmt.Sprintf("token_count: expected %d, got %d", expected.TokenCount, submitted.TokenCount))
	}
	check("root", expected.Root, submitted.Root)
	return diffs
}
