package parts

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strconv"

	ipfsnode "github.com/ipfs/go-ipfs-api"

	"github.com/rubixchain/rubixgoplatform/core/coin"
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


func IpfsAddString(heirarchicalID TokenID, ipfsClient IPFSOperation) (string, error) {
	heirarchicalIDStr := heirarchicalID.String()
	heirarchicalIDStrBuffer := bytes.NewBufferString(heirarchicalIDStr)

	return ipfsClient.Add(heirarchicalIDStrBuffer, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
}

func IpfsCatString(id string, ipfsClient IPFSOperation) (TokenID, error) {
	ipfsCatInfo, err := ipfsClient.Cat(id)
	if err != nil {
		return "", err
	}

	ipfsCatBytes, err := io.ReadAll(ipfsCatInfo)
	if err != nil {
		return "", err
	}

	return TokenID(ipfsCatBytes), nil
}

func floatModulo(a float64, b float64) (int, error) {
	expVal := math.Pow10(coin.MaxSupportedDecimalPlaces)

	aInt := int(math.Round(a * expVal))
	bInt := int(math.Round(b * expVal))

	if bInt == 0 {
		return 0,
			fmt.Errorf("floatModulo: right operand found to be zero")
	}

	modInt := aInt % bInt
	return modInt, nil
}

func GetIPFSHashFromHeirarchicalID(ipfsOpfs IPFSOperation, heirarchicalID string) (string, error) {
	heirarchicalIDBuffer := bytes.NewBufferString(heirarchicalID)
	return ipfsOpfs.Add(heirarchicalIDBuffer, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
}

