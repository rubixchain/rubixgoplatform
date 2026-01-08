package parts

import (
	"strconv"

	"github.com/rubixchain/rubixgoplatform/token"
)

type TransferResult struct {
	Transferred []string
	Operations  []Operation
}

type Operation struct {
	Type        string
	Level       int
	Count       int
	Description string
}

func floatToInt(inputNum int) string {
	return strconv.FormatInt(int64(inputNum), 10)
}

func floatMultiply(floatVal float64, multiplier int) float64 {
	multiplierFloat := float64(multiplier)

	return floatVal * multiplierFloat
}

func getMinimum(x int, y int) int {
	if x < y {
		return x
	}

	return y
}

func GetParentTokenType(isTestnet bool) int {
	if isTestnet {
		return token.TestTokenType
	}
	return token.RBTTokenType
}

func GetChildTokenType(isTestnet bool) int {
	if isTestnet {
		return token.TestPartTokenType
	}
	return token.PartTokenType
}
