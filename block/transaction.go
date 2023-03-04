package block

import (
	"strconv"

	"github.com/rubixchain/rubixgoplatform/util"
)

// ----------TransInfo--------------------------
// {
//   "1" : SenderDID   : string
//   "2" : ReceiverDID : string
//   "3" : Comment     : string
//   "4" : TID         : string
//   "5" : Block       : []byte
//   "6" : Tokens      : map[string]TransToken
// }
// ----------TransToken--------------------------
// {
//   "1" : TokenType       : int
//   "2" : PledgedToken    : string
//   "3" : PledgedDID      : string
//   "4" : BlockNumber     : string
//   "5" : PreviousBlockID : string
// }

const (
	TISenderDIDKey   string = "1"
	TIReceiverDIDKey string = "2"
	TICommentKey     string = "3"
	TITIDKey         string = "4"
	TIBlockKey       string = "5"
	TITokensKey      string = "6"
)

const (
	TTTokenTypeKey       string = "1"
	TTPledgedTokenKey    string = "2"
	TTPledgedDIDKey      string = "3"
	TTBlockNumberKey     string = "4"
	TTPreviousBlockIDKey string = "5"
)

type TransTokens struct {
	Token        string `json:"token"`
	TokenType    int    `json:"tokenType"`
	PledgedToken string `json:"pledgedToken"`
	PledgedDID   string `json:"pledgedDID"`
}

type TransInfo struct {
	SenderDID   string        `json:"senderDID"`
	ReceiverDID string        `json:"receiverDID"`
	Comment     string        `json:"comment"`
	TID         string        `json:"tid"`
	Block       []byte        `json:"block"`
	Tokens      []TransTokens `json:"tokens"`
}

func newTransToken(b *Block, tt *TransTokens) map[string]interface{} {
	if tt.Token == "" {
		return nil
	}
	nttb := make(map[string]interface{})
	nttb[TTTokenTypeKey] = tt.TokenType
	if tt.PledgedToken != "" {
		nttb[TTPledgedTokenKey] = tt.PledgedToken
	}
	if tt.PledgedDID != "" {
		nttb[TTPledgedDIDKey] = tt.PledgedDID
	}
	if b == nil {
		nttb[TTBlockNumberKey] = "0"
		nttb[TTPreviousBlockIDKey] = ""
	} else {
		bn, err := b.GetBlockNumber(tt.Token)
		if err != nil {
			return nil
		}
		bn++
		bid, err := b.GetBlockID(tt.Token)
		if err != nil {
			return nil
		}
		nttb[TTBlockNumberKey] = strconv.FormatUint(bn, 10)
		nttb[TTPreviousBlockIDKey] = bid
	}
	return nttb
}

func newTransInfo(ctcb map[string]*Block, ti *TransInfo) map[string]interface{} {
	ntib := make(map[string]interface{})
	if ti.Tokens == nil || len(ti.Tokens) == 0 {
		return nil
	}
	if ti.SenderDID != "" {
		ntib[TISenderDIDKey] = ti.SenderDID
	}
	if ti.ReceiverDID != "" {
		ntib[TIReceiverDIDKey] = ti.ReceiverDID
	}
	if ti.Comment != "" {
		ntib[TICommentKey] = ti.Comment
	}
	if ti.TID != "" {
		ntib[TITIDKey] = ti.TID
	}
	if ti.Block != nil {
		ntib[TIBlockKey] = ti.Block
	}
	nttbs := make(map[string]interface{})
	for _, tt := range ti.Tokens {
		b := ctcb[tt.Token]
		nttb := newTransToken(b, &tt)
		if nttb == nil {
			return nil
		}
		nttbs[tt.Token] = nttb
	}
	ntib[TITokensKey] = nttbs
	return ntib
}

func (b *Block) getTrasnInfoString(key string) string {
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return ""
	}
	si := util.GetFromMap(tim, key)
	return util.GetString(si)
}
