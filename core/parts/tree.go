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
}

func BuildDenomTree(tokens []wallet.Token, ownerDID string) (*DenomTree, error) {
	denomTree := &DenomTree{
		OwnerDID: ownerDID,
	}

	for i := range tokens {
		hierarchicalId, err := IndexedToHierarchical(tokens[i].TokenID)
		if err != nil {
			return nil, fmt.Errorf("BuildTokenTree: failed to convert indexed ID to hierarchical ID: %v", err)
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
