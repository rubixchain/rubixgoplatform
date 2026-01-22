package parts

// Maximum supported decimal places for Part tokens
const MaxSupportedDecimalPlaces int = 3


// Borrowed from Core package to avoid cyclic imports
const (
	RBTTokenType int = iota
	SmartContractTokenType
	NFTTokenType
	FTTokenType
)