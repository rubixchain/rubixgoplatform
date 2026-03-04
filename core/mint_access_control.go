package core

import "fmt"

// MintAccessRange defines the Level 1 token number range a DID is authorized to mint.
// All tokens are Level 1, so we only specify token numbers (not level).
type MintAccessRange struct {
	DID              string
	StartTokenNumber int // Inclusive - first Level 1 token number this DID can mint
	EndTokenNumber   int // Inclusive - last Level 1 token number this DID can mint
}

// AllowedMinters defines the authorized DIDs and their Level 1 token number ranges.
// Level 1 contains 4,300,000 tokens (token numbers 1 to 4,300,000).
// Distribution:
//   - DIDs 1-10: 400,000 tokens each
//   - DID 11: 300,000 tokens
//
// IMPORTANT: Replace placeholder DIDs with actual DIDs before deployment.
// This is a security-critical configuration - only DIDs listed here can mint tokens.
var AllowedMinters = []MintAccessRange{
	{
		DID:              "DID_1_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 1,
		EndTokenNumber:   400000, // 400,000 tokens
	},
	{
		DID:              "DID_2_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 400001,
		EndTokenNumber:   800000, // 400,000 tokens
	},
	{
		DID:              "DID_3_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 800001,
		EndTokenNumber:   1200000, // 400,000 tokens
	},
	{
		DID:              "DID_4_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 1200001,
		EndTokenNumber:   1600000, // 400,000 tokens
	},
	{
		DID:              "DID_5_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 1600001,
		EndTokenNumber:   2000000, // 400,000 tokens
	},
	{
		DID:              "DID_6_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 2000001,
		EndTokenNumber:   2400000, // 400,000 tokens
	},
	{
		DID:              "DID_7_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 2400001,
		EndTokenNumber:   2800000, // 400,000 tokens
	},
	{
		DID:              "DID_8_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 2800001,
		EndTokenNumber:   3200000, // 400,000 tokens
	},
	{
		DID:              "DID_9_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 3200001,
		EndTokenNumber:   3600000, // 400,000 tokens
	},
	{
		DID:              "DID_10_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 3600001,
		EndTokenNumber:   4000000, // 400,000 tokens
	},
	{
		DID:              "DID_11_PLACEHOLDER", // Replace with actual DID
		StartTokenNumber: 4000001,
		EndTokenNumber:   4300000, // 300,000 tokens (last DID gets remaining tokens)
	},
}

// GetDIDForTokenNumber returns the DID authorized to mint a specific Level 1 token number.
// This is a reverse lookup function: given a token number, find which DID owns it.
//
// Example:
//   did, _ := GetDIDForTokenNumber(150000)   // Returns DID_1_PLACEHOLDER
//   did, _ := GetDIDForTokenNumber(1500000)  // Returns DID_4_PLACEHOLDER
//   did, _ := GetDIDForTokenNumber(4200000)  // Returns DID_11_PLACEHOLDER
//
// Returns (did, nil) if token number is within an authorized range, or ("", error) otherwise.
// Performance: O(11) - effectively constant time for 11 DIDs.
func GetDIDForTokenNumber(tokenNumber int) (string, error) {
	for _, minter := range AllowedMinters {
		if tokenNumber >= minter.StartTokenNumber && tokenNumber <= minter.EndTokenNumber {
			return minter.DID, nil
		}
	}
	return "", fmt.Errorf("no authorized DID found for Level 1 token number %d", tokenNumber)
}

// ValidateDIDOwnsTokenNumber checks if a specific DID is authorized to mint a specific Level 1 token number.
// This is useful for quick validation checks during minting.
//
// Example:
//   ok := ValidateDIDOwnsTokenNumber("DID_1_PLACEHOLDER", 150000)  // Returns true
//   ok := ValidateDIDOwnsTokenNumber("DID_1_PLACEHOLDER", 500000)  // Returns false
//
// Returns true if the DID is authorized for this token number, false otherwise.
// Performance: O(11) - effectively constant time.
func ValidateDIDOwnsTokenNumber(did string, tokenNumber int) bool {
	for _, minter := range AllowedMinters {
		if minter.DID == did {
			return tokenNumber >= minter.StartTokenNumber && tokenNumber <= minter.EndTokenNumber
		}
	}
	return false
}

// ValidateDIDOwnsTokenRange validates that a DID is authorized to mint an entire range of Level 1 token numbers.
// This is the primary validation function used in mintTokens() before starting the minting process.
//
// It checks that:
//  1. The DID exists in the allowed minters list
//  2. The entire range [startTokenNumber, endTokenNumber] is within the DID's authorized range
//
// Example:
//   err := ValidateDIDOwnsTokenRange("DID_1_PLACEHOLDER", 1, 100000)      // Returns nil (valid)
//   err := ValidateDIDOwnsTokenRange("DID_1_PLACEHOLDER", 1, 500000)      // Returns error (exceeds range)
//   err := ValidateDIDOwnsTokenRange("DID_1_PLACEHOLDER", 400001, 500000) // Returns error (wrong range)
//
// Returns nil if access is granted, error with detailed message otherwise.
// Performance: O(11) - effectively constant time.
func ValidateDIDOwnsTokenRange(did string, startTokenNumber, endTokenNumber int) error {
	if startTokenNumber > endTokenNumber {
		return fmt.Errorf("invalid range: startTokenNumber (%d) > endTokenNumber (%d)", startTokenNumber, endTokenNumber)
	}

	for _, minter := range AllowedMinters {
		if minter.DID == did {
			// Check if requested range is fully within authorized range
			if startTokenNumber >= minter.StartTokenNumber && endTokenNumber <= minter.EndTokenNumber {
				return nil
			}
			return fmt.Errorf(
				"DID %s cannot mint Level 1 tokens [%d, %d]. Allowed range: [%d, %d]",
				did, startTokenNumber, endTokenNumber,
				minter.StartTokenNumber, minter.EndTokenNumber,
			)
		}
	}
	return fmt.Errorf("DID %s is not authorized to mint tokens", did)
}

// GetTokenRangeForDID returns the Level 1 token number range assigned to a specific DID.
// This is useful for informational purposes and API queries.
//
// Example:
//   start, end, _ := GetTokenRangeForDID("DID_5_PLACEHOLDER")  // Returns: 1600001, 2000000, nil
//
// Returns (startTokenNumber, endTokenNumber, nil) if DID is found, or (0, 0, error) otherwise.
func GetTokenRangeForDID(did string) (int, int, error) {
	for _, minter := range AllowedMinters {
		if minter.DID == did {
			return minter.StartTokenNumber, minter.EndTokenNumber, nil
		}
	}
	return 0, 0, fmt.Errorf("DID %s is not in the allowed minters list", did)
}

// ListAllMintRanges returns a formatted string with all DIDs and their authorized Level 1 token ranges.
// This is useful for debugging, logging, and administrative purposes.
//
// Example output:
//   Level 1 Token Minting Authorization
//   ====================================
//
//   DID #1: DID_1_PLACEHOLDER
//     Token Range: [1, 400,000]
//     Total Tokens: 400,000
//
//   DID #2: DID_2_PLACEHOLDER
//     Token Range: [400,001, 800,000]
//     Total Tokens: 400,000
//   ...
func ListAllMintRanges() string {
	result := "Level 1 Token Minting Authorization\n"
	result += "====================================\n\n"

	for i, minter := range AllowedMinters {
		tokenCount := minter.EndTokenNumber - minter.StartTokenNumber + 1
		result += fmt.Sprintf("DID #%d: %s\n", i+1, minter.DID)
		result += fmt.Sprintf("  Token Range: [%s, %s]\n",
			formatNumber(minter.StartTokenNumber),
			formatNumber(minter.EndTokenNumber))
		result += fmt.Sprintf("  Total Tokens: %s\n\n", formatNumber(tokenCount))
	}

	totalTokens := 4300000
	result += fmt.Sprintf("Total Level 1 Tokens: %s\n", formatNumber(totalTokens))

	return result
}

// formatNumber formats an integer with comma thousands separators for readability.
// Example: 1500000 -> "1,500,000"
func formatNumber(n int) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	str := fmt.Sprintf("%d", n)
	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}
