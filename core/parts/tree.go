package parts

import (
	"sort"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

type DenomTreeNode struct {
	TokenID TokenID
	Token   models.Token
	IsLeaf  bool
}

type DenomTree struct {
	Leaves   []*DenomTreeNode
	OwnerDID string
}

func BuildDenomTree(tokens []models.Token, ownerDID string) (*DenomTree, error) {
	denomTree := &DenomTree{
		OwnerDID: ownerDID,
	}

	for i := range tokens {
		node := &DenomTreeNode{
			TokenID: TokenID(tokens[i].TokenID),
			Token:   tokens[i],
			IsLeaf:  true,
		}

		denomTree.Leaves = append(denomTree.Leaves, node)
	}

	sort.Slice(denomTree.Leaves, func(i, j int) bool {
		return denomTree.Leaves[i].TokenID.LexicalCompare(
			denomTree.Leaves[j].TokenID,
		) < 0
	})

	return denomTree, nil
}
