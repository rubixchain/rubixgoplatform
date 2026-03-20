package util

import (
	"encoding/hex"

	"golang.org/x/crypto/sha3"

	"github.com/rubixchain/rubixgoplatform/constants"
)

// StrToHex decodes a hex-encoded string to a raw byte slice.
// Returns nil if the input is not valid hex.
func StrToHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

func CalculateHash(data []byte, method string) []byte {
	switch method {
	case constants.HashAlgorithm_SHA3_256:
		h := sha3.New256()
		h.Write(data)
		return h.Sum(nil)
	default:
		return nil
	}
}
