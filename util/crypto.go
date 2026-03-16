package util

import (
	"crypto/sha3"

	"github.com/rubixchain/rubixgoplatform/constants"
)
 
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