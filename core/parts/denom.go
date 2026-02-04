package parts

import (
	"fmt"
	"math"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
)

// GetMaxDenomTreeLevel gets the max level of a Denom Tree
func GetMaxDenomTreeLevel() int {
	return (2 * constants.MaxSupportedDecimalPlaces) + 1
}

func getLowestPossibleDenom() float64 {
	return rubixmath.FloatPrecision(math.Pow10(-constants.MaxSupportedDecimalPlaces))
}

// getSubtreeSize calculates the total number of nodes in a subtree starting at the given depth
// Includes the node itself plus all its descendants
func getSubtreeSize(depth int) int {
	maxDepthWithoutRoot := GetMaxDenomTreeLevel() - 1
	if depth >= maxDepthWithoutRoot {
		return 1 // Leaf node
	}

	// Alternating pattern: even depths divide by 2, odd depths divide by 5
	var numChildren int
	if depth%2 == 0 {
		numChildren = 2
	} else {
		numChildren = 5
	}

	childSize := getSubtreeSize(depth+1)
	return 1 + numChildren*childSize
}

func IndexedToHierarchical(indexedToken string) (TokenID, error) {
	parts := strings.Split(indexedToken, "_")
	if len(parts) < 3 {
		if len(parts) == 2 {
			// This is the whole token case
			return TokenID(indexedToken), nil
		}
		return "", fmt.Errorf("invalid indexed token format: expected at least 3 parts, got %d", len(parts))
	}

	// Extract prefix (TokenLevel_TokenNumber)
	prefix := parts[0] + "_" + parts[1]

	// Parse the indexed number
	var tokenId int
	_, err := fmt.Sscanf(parts[2], "%d", &tokenId)
	if err != nil {
		return "", fmt.Errorf("invalid indexed number: %s", parts[2])
	}

	// Convert to hierarchical path
	path := GetTokenPath(tokenId)

	// Return full hierarchical form
	return TokenID(prefix + "_" + path), nil
}

// GetTokenPath converts a tokenId to its path representation
// Returns a string like "1_2" (just the hierarchical path portion)
func GetTokenPath(tokenId int) string {
	maxDepthWithoutRoot := GetMaxDenomTreeLevel() - 1
	// Root node
	if tokenId == 0 {
		return "0"
	}

	path := make([]int, maxDepthWithoutRoot)
	remaining := tokenId

	for depth := 0; depth < maxDepthWithoutRoot; depth++ {
		// Determine number of children at this depth
		var numChildren int
		if depth%2 == 0 {
			numChildren = 2
		} else {
			numChildren = 5
		}

		childSubtreeSize := getSubtreeSize(depth+1)

		// Find which child this tokenId belongs to
		for child := 1; child <= numChildren; child++ {
			if remaining <= childSubtreeSize {
				path[depth] = child
				remaining -= 1 // Account for the child node itself
				break
			}
			remaining -= childSubtreeSize
		}

		if remaining == 0 {
			break
		}
	}

	// Find the last non-zero element to trim trailing zeros
	lastNonZero := 0
	for i := 0; i < maxDepthWithoutRoot; i++ {
		if path[i] != 0 {
			lastNonZero = i
		}
	}

	// Convert path to string (only up to last non-zero element)
	pathStr := make([]string, lastNonZero+1)
	for i := 0; i <= lastNonZero; i++ {
		pathStr[i] = fmt.Sprintf("%d", path[i])
	}

	return strings.Join(pathStr, "_")
}


func HeirarchicalToIndexed(heirarchicalID TokenID) (string, error) {
	parts := strings.Split(heirarchicalID.String(), "_")
	if len(parts) < 3 {
		if len(parts) == 2 {
			// This is the whole token case
			return string(heirarchicalID), nil
		}
		return "", fmt.Errorf("invalid hierarchical token format: expected at least 3 parts, got %d", len(parts))
	}

	// Extract prefix (TokenLevel_TokenNumber)
	prefix := parts[0] + "_" + parts[1]

	// Extract hierarchical path (everything after prefix)
	pathParts := parts[2:]
	pathStr := strings.Join(pathParts, "_")

	indexedToken, err := GetTokenIdFromPath(pathStr)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s_%d", prefix, indexedToken), nil
}

// GetTokenIdFromPath converts a path string back to its tokenId
// Takes a path like "34_2345_1_2" (with prefix) or "1_2" (without prefix)
// Auto-detects and strips the prefix if present (first 2 parts if first part > 2)
func GetTokenIdFromPath(pathStr string) (int, error) {
	maxDepthWithoutRoot := GetMaxDenomTreeLevel() - 1

	parts := strings.Split(pathStr, "_")

	// Auto-detect prefix: check if first part is a valid position (1 or 2)
	// If not, assume first 2 parts are prefix (NetworkLevel_TokenNumber)
	if len(parts) >= 3 {
		var firstVal int
		fmt.Sscanf(parts[0], "%d", &firstVal)
		if firstVal > 2 {
			// First part is not a valid position, strip prefix
			parts = parts[2:]
			pathStr = strings.Join(parts, "_")
		}
	}

	// Root case
	if pathStr == "0" {
		return 0, fmt.Errorf("unexpected case: whole token found for path: %v", pathStr)
	}

	if len(parts) > maxDepthWithoutRoot {
		return 0, fmt.Errorf("path length %d exceeds MaxDepth %d", len(parts), maxDepthWithoutRoot)
	}

	path := make([]int, len(parts))
	for i := 0; i < len(parts); i++ {
		_, err := fmt.Sscanf(parts[i], "%d", &path[i])
		if err != nil {
			return 0, fmt.Errorf("invalid number at depth %d: %s", i, parts[i])
		}
	}

	tokenId := 0

	for depth := 0; depth < len(path); depth++ {
		child := path[depth]

		var numChildren int
		if depth%2 == 0 {
			numChildren = 2
		} else {
			numChildren = 5
		}

		if child < 1 || child > numChildren {
			return 0, fmt.Errorf("invalid child %d at depth %d", child, depth)
		}

		childSubtreeSize := getSubtreeSize(depth+1)

		// preorder visit of this node
		tokenId += 1

		// skip previous siblings
		tokenId += (child - 1) * childSubtreeSize
	}

	return tokenId, nil
}
