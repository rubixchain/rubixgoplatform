package model

type TCDumpRequest struct {
	Token   string `json:"token"`
	BlockID string `json:"block_id"`
}

type TCDumpReply struct {
	BasicResponse
	NextBlockID string   `json:"next_block_id"`
	Blocks      [][]byte `json:"blocks"`
}

type GetFTTokenChainReply struct {
	BasicResponse
	TokenChainData string
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
