package wallet

import (
	"fmt"

	"github.com/syndtr/goleveldb/leveldb/util"
	tkn "github.com/rubixchain/rubixgoplatform/token"
)

func (w *Wallet) GetAllTokenKeys(tokenType int) ([]string, error) {
	var tokenKeys []string

	db := w.getChainDB(tokenType)
	if db == nil {
		return tokenKeys, fmt.Errorf("failed get all blocks, invalid token type")
	}

	iter := db.NewIterator(util.BytesPrefix([]byte(tcsPrefixForNodeSync(tokenType))), nil)
	defer iter.Release()
	for iter.Next() {
		key := string(iter.Key())
		tokenKeys = append(tokenKeys, key)
	}

	return tokenKeys, nil
}


func tcsPrefixForNodeSync(tokenType int) string {
	tt := "wt"
	switch tokenType {
	case tkn.RBTTokenType:
		tt = WholeTokenType
	case tkn.PartTokenType:
		tt = PartTokenType
	case tkn.TestPartTokenType:
		tt = TestPartTokenType
	case tkn.NFTTokenType:
		tt = NFTType
	case tkn.TestNFTTokenType:
		tt = TestNFTType
	case tkn.TestTokenType:
		tt = TestTokenType
	case tkn.DataTokenType:
		tt = DataTokenType
	case tkn.SmartContractTokenType:
		tt = SmartContractTokenType
	}
	return tt + "-"
}
