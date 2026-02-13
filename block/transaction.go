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
//   "7" : BlockIDs    : []string
// }
// ----------TransToken--------------------------
// {
//   "1" : TokenType       : int
//   "2" : PledgedToken    : string  depreciated not used
//   "3" : PledgedDID      : string  depreciated not used
//   "4" : BlockNumber     : string
//   "5" : PreviousBlockID : string
//   "6" : UnpledgedID     : string
// }
// ----------PledgeDetails------------------------
// {
//   "1" : Token           : string
//   "2" : TokenType       : int
//   "3" : TokenBlockID    : string
// }

const (
	TISenderDIDKey      string = "senderDID"
	TICommentKey        string = "comment"
	TITIDKey            string = "txnID"
	TIBlockKey          string = "block"
	TITokensKey         string = "transTokens"
	TIRefIDKey          string = "refID"
	TIDeployerDIDKey    string = "deployerDID"
	TIExecutorDIDKey    string = "executorDID"
	TICommitedTokensKey string = "commitedTokens"
	TIPinningDIDKey     string = "pinningDID"
)

const (
	TTTokenTypeKey       string = "tokenType"
	TTBlockNumberKey     string = "blockNumber"
	TTPreviousBlockIDKey string = "previousBlockID"
	TTUnpledgedIDKey     string = "unpledgedID"
	TTCommitedDIDKey     string = "commitedDID"
)

const (
	PDTokenKey        string = "pledgeToken"
	PDTokenTypeKey    string = "tokenType"
	PDTokenBlockIDKey string = "tokenBlockID"
)

type TransTokens struct {
	Token       string `json:"token"`
	TokenType   int    `json:"tokenType"`
	UnplededID  string `json:"unpledgedID"`
	CommitedDID string `json:"commitedDID"`
}

type TransInfo struct {
	SenderDID      string        `json:"senderDID"`
	Comment        string        `json:"comment"`
	TID            string        `json:"txnID"`
	Block          []byte        `json:"block"`
	RefID          string        `json:"refID"`
	Tokens         []TransTokens `json:"transTokens"`
	DeployerDID    string        `json:"deployerDID"`
	ExecutorDID    string        `json:"executorDID"`
	PinningNodeDID string        `json:"pinningNodeDID"`
}

func newTransToken(b *Block, tt *TransTokens) map[string]interface{} {
	if tt.Token == "" {
		return nil
	}
	nttb := make(map[string]interface{})
	nttb[TTTokenTypeKey] = tt.TokenType
	// pledged detials moved out of trans token
	if tt.UnplededID != "" {
		nttb[TTUnpledgedIDKey] = tt.UnplededID
	}
	if tt.CommitedDID != "" {
		nttb[TTCommitedDIDKey] = tt.CommitedDID
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
	if ti.PinningNodeDID != "" {
		ntib[TIPinningDIDKey] = ti.PinningNodeDID
	}
	if ti.DeployerDID != "" {
		ntib[TIDeployerDIDKey] = ti.DeployerDID
	}
	if ti.ExecutorDID != "" {
		ntib[TIExecutorDIDKey] = ti.ExecutorDID
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
	if ti.RefID != "" {
		ntib[TIRefIDKey] = ti.RefID
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

func (b *Block) GetTransBlock() []byte {
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return nil
	}
	si := util.GetFromMap(tim, TIBlockKey)
	return util.GetBytes(si)
}

func (b *Block) GetRefID() string {
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return ""
	}
	si := util.GetFromMap(tim, TIRefIDKey)
	return util.GetString(si)
}

func newPledgeDetails(pds []PledgeDetail) map[string]interface{} {
	if len(pds) == 0 {
		return nil
	}
	npds := make(map[string]interface{})
	for _, pd := range pds {
		npd, ok := npds[pd.DID].([]map[string]interface{})
		if !ok {
			npd = make([]map[string]interface{}, 0)
		}
		np := make(map[string]interface{})
		np[PDTokenKey] = pd.Token
		np[PDTokenTypeKey] = pd.TokenType
		np[PDTokenBlockIDKey] = pd.TokenBlockID
		npd = append(npd, np)
		npds[pd.DID] = npd
	}
	return npds
}
