package parts

import (
	"fmt"
	"sort"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

type DenomTreeNode struct {
	HierarchicalID TokenID
	Token          wallet.Token
	IsLeaf         bool
}

type DenomTree struct {
	Leaves   []*DenomTreeNode
	OwnerDID string
	ipfsOps  IPFSOperation
}

func BuildDenomTree(tokens []wallet.Token, ownerDID string, ipfsClient IPFSOperation) (*DenomTree, error) {
	denomTree := &DenomTree{
		OwnerDID: ownerDID,
		ipfsOps:  ipfsClient,
	}

	for i := range tokens {
		hierarchicalId, err := IpfsCatString(tokens[i].TokenID, ipfsClient)
		if err != nil {
			return nil, fmt.Errorf("BuildTokenTree: failed to get the heirarchical IF")
		}
		node := &DenomTreeNode{
			HierarchicalID: hierarchicalId,
			Token:          tokens[i],
			IsLeaf:         true,
		}

		denomTree.Leaves = append(denomTree.Leaves, node)
	}

	sort.Slice(denomTree.Leaves, func(i, j int) bool {
		return denomTree.Leaves[i].HierarchicalID.LexicalCompare(
			denomTree.Leaves[j].HierarchicalID,
		) < 0
	})

	return denomTree, nil
}
