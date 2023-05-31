package model

type TCDumpRequest struct {
	Token   string
	BlockID string
}

type TCDumpReply struct {
	BasicResponse
	NextBlockID string   `json:"next_block_id"`
	Blocks      [][]byte `json:"blocks"`
}

type TCRemoveRequest struct {
	Token string
}

type TCRemoveReply struct {
	BasicResponse
}
