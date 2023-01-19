package wallet

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/core/util"
)

func TestTemp(t *testing.T) {

	var tc []map[string]interface{}
	err := util.ParseJsonFile("d:/test.json", &tc)
	if err != nil {
		t.Fatalf("Failed to parse file, %s", err.Error())
	}
	l := len(tc)
	if l == 0 {
		t.Fatalf("Invalid array")
	}
	delete(tc[l-1], "hash")
	delete(tc[l-1], "pvtShareBits")
	str := util.CalcTokenChainHash(tc)
	util.FileWrite("d:/test_temp.json", []byte(str))
	hash := util.CalculateHash([]byte(str), "SHA3-256")
	if hash != nil {
		t.Fatalf("Failed to parse file, %s", err.Error())
	}
}
