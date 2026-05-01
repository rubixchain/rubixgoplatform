package util

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
)

var TreeLevelRanges = computeTreeLevelRanges()

func LevelToDenom(level int) (float64, error) {
	if level < 0 {
		return 0, fmt.Errorf("LevelToDenom: level cannot be non negative, provided level: %v", level)
	}

	k := level / 2
	if level%2 == 0 {
		return math.Pow(10, -float64(k)), nil
	}
	return 5 * math.Pow(10, -float64(k+1)), nil
}

func DenomToLevel(denom float64) (int, error) {
	if denom <= 0 {
		return 0, fmt.Errorf("DenomToLevel: denom must be positive, provided denom: %v", denom)
	}

	exp := math.Floor(math.Log10(denom))
	mantissa := denom / math.Pow(10, exp)

	const eps = 1e-9
	if math.Abs(mantissa-1.0) < eps {
		return int(-2 * exp), nil
	}
	if math.Abs(mantissa-5.0) < eps {
		return int(-2*exp - 1), nil
	}

	return 0, fmt.Errorf("DenomToLevel: denom %v is not part of the supported denomination set", denom)
}

func GetMaxLevel() int {
	return (2 * constants.MaxSupportedDecimalPlaces) + 1
}

// computeTreeLevelRanges dynamically builds the [min, max] part-index range
// for each level of the token-subdivision tree.
//
// The tree subdivides 1 token down to maxDecimalPlaces decimal places:
//   - Even-level nodes have 2 children (÷2 subdivision)
//   - Odd-level nodes  have 5 children (÷5 subdivision)
//
// The maximum depth is derived from maxDecimalPlaces by simulating the
// cumulative subdivision until the value reaches 10^-maxDecimalPlaces.
//
// Example with maxDecimalPlaces=3:
//
//	L0=1.0 ; L1=0.5(÷2) ; L2=0.1(÷5) ; L3=0.05(÷2) ;
//	L4=0.01(÷5) ; L5=0.005(÷2) ; L6=0.001(÷5)  ; maxDepth=6
func computeTreeLevelRanges() [][2]int {
	// smallest unit = 10^-maxDecimalPlaces (stored scaled to avoid floats)
	// we work in integer arithmetic: value is scaled by 10^maxDecimalPlaces
	// so 1 token = 10^maxDecimalPlaces units, smallest unit = 1
	scaledValue := 1
	for i := 0; i < constants.MaxSupportedDecimalPlaces; i++ {
		scaledValue *= 10
	}
	smallestUnit := 1 // = 10^0 after scaling

	// Walk down the tree, dividing at each level until we reach smallestUnit.
	// divisors[L] is the number of children of a node at level L.
	divisors := []int{}
	current := scaledValue
	level := 0
	for current > smallestUnit {
		d := GetNumberOfChildren(level)
		divisors = append(divisors, d)
		current /= d
		level++
	}
	maxDepth := len(divisors) // number of levels below L0

	// Build ranges.
	// ranges[0] = L0 = {0, 0} (the whole token, single virtual node)
	ranges := make([][2]int, maxDepth+1)
	ranges[0] = [2]int{0, 0}

	nodeCount := 1 // L0 has exactly 1 node
	nextMin := 1   // part index of the first node at the next level

	for l := 1; l <= maxDepth; l++ {
		nodeCount *= divisors[l-1] // children of level l-1
		min := nextMin
		max := min + nodeCount - 1
		ranges[l] = [2]int{min, max}
		nextMin = max + 1
	}

	return ranges
}

// GetTreeLevelFromPartIndex returns the tree level (1-6) for a given part index x.
func GetTreeLevelFromPartIndex(x int) (int, error) {
	for level, r := range TreeLevelRanges {
		if x >= r[0] && x <= r[1] {
			return level, nil // levels are 1-indexed
		}
	}
	return 0, fmt.Errorf("part index %d is out of range (valid: 1 - 1332)", x)
}

// GetNumberOfChildren returns the number of children for a node at parentLevel.
// n = parentLevel % 2:  n==0 -> 2 children,  n==1 -> 5 children.
func GetNumberOfChildren(parentLevel int) int {
	if parentLevel%2 == 0 {
		return 2 // when parent level is even
	}
	return 5 // when parent level is odd
}

// get level, token number and part index from the RBT token id
func GetRbtIDElements(tokenID string) (types.RbtIDElements, error) {
	var err error
	rbtElems := types.RbtIDElements{}

	// Check if the token is a Smart Contract or NFT
	if strings.HasPrefix(tokenID, "Qm") {
		return types.RbtIDElements{}, fmt.Errorf("token ID %v is most likely a NFT or Smart Contract", tokenID)
	}

	// check if token id is ft id, by checking if the length of the id is more than legth of DID (59)
	if len(tokenID) > 59 {
		return types.RbtIDElements{}, fmt.Errorf("invalid token id format for rbt: %s, id length should be <= 15 (<max 2 digits level>_<max 7 digits token number>_<max 4 digits part index>)", tokenID)
	}

	idElems := strings.Split(tokenID, "_")
	if len(idElems) < 2 || len(idElems) > 3 { // ensure id is in proper RBT id format
		return types.RbtIDElements{}, fmt.Errorf("invalid token id format for rbt: %s, id elements should be 2 (whole) or 3 (part)", tokenID)
	}

	rbtElems.TokenLevel, err = strconv.Atoi(idElems[0])
	if err != nil {
		return types.RbtIDElements{}, fmt.Errorf("failed to convert level into int for rbt: %s, error: %v", tokenID, err)
	}
	rbtElems.TokenNumber, err = strconv.Atoi(idElems[1])
	if err != nil {
		return types.RbtIDElements{}, fmt.Errorf("failed to convert token number into int for rbt: %s, error: %v", tokenID, err)
	}

	switch len(idElems) {
	case 2:
		rbtElems.PartIndex = 0 // Case for whole token
	case 3:
		rbtElems.PartIndex, err = strconv.Atoi(idElems[2]) // Case for part token
		if err != nil {
			return types.RbtIDElements{}, fmt.Errorf("failed to convert part index into int for rbt: %s, error: %v", tokenID, err)
		}
	default:
		return types.RbtIDElements{}, fmt.Errorf("invalid token id format for rbt: %s, id elements should be 2 (whole) or 3 (part)", tokenID)
	}
	return rbtElems, nil
}

// GetTokenValueFromTokenID fetches the token value by
// looking at the IPFS content
func GetTokenValueFromTokenID(tokenID string) (float64, error) {
	// split RBT id elements
	tokenElems, err := GetRbtIDElements(tokenID)
	if err != nil {
		return rubixmath.ZeroFloat(), fmt.Errorf("GetTokenValueFromTokenID: failed to split elements of token ID %s, err: %v", tokenID, err)
	}

	// get token level
	tokenTreeLevel, err := GetTreeLevelFromPartIndex(tokenElems.PartIndex)
	if err != nil {
		return rubixmath.ZeroFloat(), fmt.Errorf("GetTokenValueFromTokenID: failed to get denom tree level of token %s, err: %v", tokenID, err)
	}

	// get token value from tree level
	return LevelToDenom(tokenTreeLevel)
}