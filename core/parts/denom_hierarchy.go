package parts

// import (
// 	"fmt"
// 	"strings"
// )

// // getSuffixProduct returns the product of numChildren(j) for j in [from, to).
// // numChildren(j) = 2 if j is even, 5 if j is odd.
// func getSuffixProduct(from, to int) int {
// 	product := 1
// 	for j := from; j < to; j++ {
// 		if j%2 == 0 {
// 			product *= 2
// 		} else {
// 			product *= 5
// 		}
// 	}
// 	return product
// }

// func IndexedToHierarchical(indexedToken string) (TokenID, error) {
// 	parts := strings.Split(indexedToken, "_")
// 	if len(parts) < 3 {
// 		if len(parts) == 2 {
// 			// This is the whole token case
// 			return TokenID(indexedToken), nil
// 		}
// 		return "", fmt.Errorf("invalid indexed token format: expected at least 3 parts, got %d", len(parts))
// 	}

// 	// Extract prefix (TokenLevel_TokenNumber)
// 	prefix := parts[0] + "_" + parts[1]

// 	// Parse the indexed number
// 	var tokenId int
// 	_, err := fmt.Sscanf(parts[2], "%d", &tokenId)
// 	if err != nil {
// 		return "", fmt.Errorf("invalid indexed number: %s", parts[2])
// 	}

// 	path := GetTokenPath(tokenId)
// 	// If path is empty, return just the prefix (whole token)
// 	if path == "" {
// 		return TokenID(prefix), nil
// 	}

// 	// Return full hierarchical form
// 	return TokenID(prefix + "_" + path), nil
// }

// // GetTokenPath converts a BFS tokenId to its hierarchical path representation.
// // Returns a string like "1_2" (just the path portion, without the token prefix).
// func GetTokenPath(tokenId int) string {
// 	maxLevel := GetMaxDenomTreeLevel() - 1

// 	// Root node (whole token)
// 	if tokenId == 0 {
// 		return ""
// 	}

// 	// Find which level this tokenId belongs to
// 	level := 0
// 	for level < maxLevel {
// 		if tokenId < getLevelStart(level+1) {
// 			break
// 		}
// 		level++
// 	}

// 	// Position within this level (0-indexed)
// 	pos := tokenId - getLevelStart(level)
// 	// Decode the path using mixed-radix arithmetic
// 	path := make([]int, level)
// 	for i := 0; i < level; i++ {
// 		sp := getSuffixProduct(i+1, level)
// 		path[i] = pos/sp + 1
// 		pos = pos % sp
// 	}
// 	// Convert path to string
// 	pathStr := make([]string, level)
// 	for i := 0; i < level; i++ {
// 		pathStr[i] = fmt.Sprintf("%d", path[i])
// 	}

// 	return strings.Join(pathStr, "_")
// }

// func HeirarchicalToIndexed(heirarchicalID TokenID) (string, error) {
// 	parts := strings.Split(heirarchicalID.String(), "_")
// 	if len(parts) < 3 {
// 		if len(parts) == 2 {
// 			// This is the whole token case
// 			return string(heirarchicalID), nil
// 		}
// 		return "", fmt.Errorf("invalid hierarchical token format: expected at least 3 parts, got %d", len(parts))
// 	}

// 	// Extract prefix (TokenLevel_TokenNumber)
// 	prefix := parts[0] + "_" + parts[1]

// 	// Extract hierarchical path (everything after prefix)
// 	pathParts := parts[2:]
// 	pathStr := strings.Join(pathParts, "_")

// 	indexedToken, err := GetTokenIdFromPath(pathStr)
// 	if err != nil {
// 		return "", err
// 	}

// 	return fmt.Sprintf("%s_%d", prefix, indexedToken), nil
// }

// // GetTokenIdFromPath converts a hierarchical path string to its BFS tokenId.
// // Takes a path like "34_2345_1_2" (with prefix) or "1_2" (without prefix).
// func GetTokenIdFromPath(pathStr string) (int, error) {
// 	maxDepthWithoutRoot := GetMaxDenomTreeLevel() - 1

// 	parts := strings.Split(pathStr, "_")

// 	// Auto-detect prefix: check if first part is a valid position (1 or 2)
// 	// If not, assume first 2 parts are prefix (NetworkLevel_TokenNumber)
// 	if len(parts) >= 3 {
// 		var firstVal int
// 		fmt.Sscanf(parts[0], "%d", &firstVal)
// 		if firstVal > 2 {
// 			// First part is not a valid position, strip prefix
// 			parts = parts[2:]
// 			pathStr = strings.Join(parts, "_")
// 		}
// 	}

// 	// Root case
// 	if pathStr == "0" {
// 		return 0, fmt.Errorf("unexpected case: whole token found for path: %v", pathStr)
// 	}

// 	if len(parts) > maxDepthWithoutRoot {
// 		return 0, fmt.Errorf("path length %d exceeds MaxDepth %d", len(parts), maxDepthWithoutRoot)
// 	}

// 	path := make([]int, len(parts))
// 	for i := 0; i < len(parts); i++ {
// 		_, err := fmt.Sscanf(parts[i], "%d", &path[i])
// 		if err != nil {
// 			return 0, fmt.Errorf("invalid number at depth %d: %s", i, parts[i])
// 		}
// 	}

// 	n := len(path)

// 	// BFS index = level_start(n) + position_within_level
// 	levelStart := getLevelStart(n)
// 	pos := 0
// 	for i := 0; i < n; i++ {
// 		child := path[i]

// 		var numChildren int
// 		if i%2 == 0 {
// 			numChildren = 2
// 		} else {
// 			numChildren = 5
// 		}

// 		if child < 1 || child > numChildren {
// 			return 0, fmt.Errorf("invalid child %d at depth %d", child, i)
// 		}

// 		sp := getSuffixProduct(i+1, n)
// 		pos += (child - 1) * sp
// 	}

// 	return levelStart + pos, nil
// }

