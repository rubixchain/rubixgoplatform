package nlss

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
)

// GetRandNumbers ...
func GetRandNumbers(num int) []byte {
	randBytes := make([]byte, num)
	if _, err := rand.Read(randBytes); err != nil {
		panic("Error in generating randnumber")
	}
	return randBytes
}

// GetRandNumber ...
func GetRandNumber(max int) int {
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic("Error in generating randnumber")
	}
	return int(nBig.Int64())
}

// ConvertBitString ...
func ConvertBitString(b string) []byte {
	var out []byte
	var str string

	for i := len(b); i > 0; i -= 8 {
		if i-8 < 0 {
			str = string(b[0:i])
		} else {
			str = string(b[i-8 : i])
		}
		v, err := strconv.ParseUint(str, 2, 8)
		if err != nil {
			panic(err)
		}
		out = append([]byte{byte(v)}, out...)
	}
	return out
}

// ConvertToBitString ...
func ConvertToBitString(data []byte) string {
	var bits string = ""
	for i := 0; i < len(data); i++ {
		bits = bits + fmt.Sprintf("%08b", data[i])
	}
	return bits
}
