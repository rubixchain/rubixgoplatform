package types

type DenomValue = float64
type DenomCount = int64

// RbtIDElements represents a node as <level>_<tokenNumber>_<partIndex>.
type RbtIDElements struct {
	Level       int
	TokenNumber int
	PartIndex   int
}

// ChildrenRange holds the first and last part index of a node's children.
type ChildrenRange struct {
	First int
	Last  int
}
