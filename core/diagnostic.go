package core

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (c *Core) DumpTokenChain(dr *model.TCDumpRequest) *model.TCDumpReply {
	ds := &model.TCDumpReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	t, err := c.w.ReadToken(dr.Token)
	if err != nil {
		ds.Message = "Failed to get token chain block, token does not exist"
		return ds
	}
	ts := RBTString
	if t.TokenValue < 1.0 {
		ts = PartString
	}
	blks, nextID, err := c.w.GetAllTokenBlocks(dr.Token, c.TokenType(ts), dr.BlockID)
	if err != nil {
		ds.Message = "Failed to get token chain block"
		return ds
	}
	ds.Status = true
	ds.Message = "Successfully got the token chain block"
	ds.Blocks = blks
	ds.NextBlockID = nextID
	return ds
}
