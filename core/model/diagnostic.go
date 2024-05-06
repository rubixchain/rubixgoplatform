package model

import "github.com/rubixchain/rubixgoplatform/core/wallet"

type NodeSyncRequest struct {
	Did string
}

type NodeSyncResponse struct {
	BasicResponse
}

type GetTokensByDIDRequest struct {
	Did string
}

type GetTokensByDIDResponse struct {
	BasicResponse
	Tokens []*wallet.Token
}

type TCDumpRequest struct {
	Token   string
	BlockID string
}

type TCDumpReply struct {
	BasicResponse
	NextBlockID string   `json:"next_block_id"`
	Blocks      [][]byte `json:"blocks"`
}

type SmartContractTokenChainDataReq struct {
	Token  string
	Latest bool
}

type SmartContractDataReply struct {
	BasicResponse
	SCTDataReply []SCTDataReply
}

type SCTDataReply struct {
	BlockNo           uint64
	BlockId           string
	SmartContractData string
}

type RegisterCallBackUrlReq struct {
	SmartContractToken string
	CallBackURL        string
}

type TCRemoveRequest struct {
	Token  string
	Latest bool
}

type TCRemoveReply struct {
	BasicResponse
}
