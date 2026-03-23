package util

import (
	"encoding/hex"

	"golang.org/x/crypto/sha3"

	"github.com/rubixchain/rubixgoplatform/constants"
)

// HexToStr encodes a raw byte slice to a lowercase hex string.
func HexToStr(b []byte) string {
	return hex.EncodeToString(b)
}

// StrToHex decodes a hex-encoded string to a raw byte slice.
// Returns nil if the input is not valid hex.
func StrToHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

// CalculateHashString computes a hash of data using the given method and returns the hex-encoded result.
func CalculateHashString(data string, method string) string {
	h := CalculateHash([]byte(data), method)
	if h == nil {
		return ""
	}
	return HexToStr(h)
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
