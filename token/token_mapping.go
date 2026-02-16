package token

import "fmt"

const FaucetName = "faucettestrbt"

// LocalTestTokenLevelBase is the starting level for local test tokens. Level 10000
// corresponds to TokenMap level 0 (56 tokens), 10001 to level 1 (4300000 tokens), etc.
const LocalTestTokenLevelBase = 10000

// TokenMap maps RBT token level to Tokens Awarded per the official RBT Tokenomics sheet.
// Ref: https://learn.rubix.net/docs/tools-downloads/tokenomics
var TokenMap = map[int]int{
	0:  56,
	1:  4300000,
	2:  2425000,
	3:  2303750,
	4:  2188563,
	5:  2079134,
	6:  1975178,
	7:  1876419,
	8:  1782598,
	9:  1693468,
	10: 1608795,
	11: 1528355,
	12: 1451937,
	13: 1379340,
	14: 1310373,
	15: 1244855,
	16: 1182612,
	17: 1123481,
	18: 1067307,
	19: 1013942,
	20: 963245,
	21: 915082,
	22: 869328,
	23: 825862,
	24: 784569,
	25: 745340,
	26: 708073,
	27: 672670,
	28: 639036,
	29: 607084,
	30: 576730,
	31: 547894,
	32: 520499,
	33: 494474,
	34: 469750,
	35: 446263,
	36: 423950,
	37: 402752,
	38: 382615,
	39: 363484,
	40: 345310,
	41: 328044,
	42: 311642,
	43: 296060,
	44: 281257,
	45: 267194,
	46: 253834,
	47: 241143,
	48: 229085,
	49: 217631,
	50: 206750,
	51: 196412,
	52: 186592,
	53: 177262,
	54: 168399,
	55: 159979,
	56: 151980,
	57: 144381,
	58: 137162,
	59: 130304,
	60: 117273,
	61: 105546,
	62: 94992,
	63: 85492,
	64: 76943,
	65: 69249,
	66: 62324,
	67: 56092,
	68: 50482,
	69: 45434,
	70: 40891,
	71: 36802,
	72: 33121,
	73: 29809,
	74: 26828,
	75: 24146,
	76: 21731,
	77: 19558,
	78: 17602,
}

// GetTokenLevelAndNumberForGlobalIndex maps a global token index to the
// token level (10000 + TokenMap level) and the token number within that level.
// E.g. global index 1-56 → level 10000, numbers 1-56; 57-4300056 → level 10001, numbers 1-4300000.
func GetTokenLevelAndNumberForGlobalIndex(globalIndex int) (tokenLevel int, numInLevel int, err error) {
	if globalIndex < 1 {
		return 0, 0, fmt.Errorf("global index must be >= 1, got %d", globalIndex)
	}
	cumulative := 0
	for mapLevel := 0; ; mapLevel++ {
		maxCount, ok := TokenMap[mapLevel]
		if !ok {
			return 0, 0, fmt.Errorf("global index %d exceeds max token levels", globalIndex)
		}
		if globalIndex <= cumulative+maxCount {
			tokenLevel = LocalTestTokenLevelBase + mapLevel
			numInLevel = globalIndex - cumulative
			return tokenLevel, numInLevel, nil
		}
		cumulative += maxCount
	}
}

type FaucetToken struct {
	TokenLevel         int    `json:"token_level"`
	FaucetID           string `json:"faucet_id"`
	CurrentTokenNumber int    `json:"current_token_number"`
	TotalCount         int    `json:"total_count"`
}
