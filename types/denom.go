package types

type DenomValue = float64
type DenomCount = int64

// RbtIDElements represents a node as <level>_<tokenNumber>_<globalIndex>.
type RbtIDElements struct {
	Level       int
	TokenNumber int
	GlobalIndex int
}

// ChildrenRange holds the first and last global index of a node's children.
type ChildrenRange struct {
	First int
	Last  int
}