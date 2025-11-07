package grpcserver

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/protos"
)

func (rn *RubixNative) GenerateRBT(ctx context.Context, in *protos.GenerateReq) (*protos.BasicReponse, error) {
	c, tkn, err := rn.getClient(ctx, true)
	if err != nil {
		return nil, err
	}
	br, err := c.GenerateTestRBT(int(in.TokenCount), rn.c.GetTokenDID(tkn))
	if err != nil {
		return nil, err
	}
	return rn.basicResponse(br)
}

func (rn *RubixNative) TransferRBT(ctx context.Context, in *protos.TransferRBTReq) (*protos.BasicReponse, error) {
	c, tkn, err := rn.getClient(ctx, true)
	if err != nil {
		return nil, err
	}
	rt := &model.RBTTransferRequest{
		Receiver:   in.Receiver,
		Sender:     rn.c.GetPeerID() + "." + rn.c.GetTokenDID(tkn),
		TokenCount: in.TokenCount,
		Type:       int(in.Type),
		Comment:    in.Comment,
	}
	br, err := c.TransferRBT(rt)
	if err != nil {
		return nil, err
	}
	return rn.basicResponse(br)
}

func (rn *RubixNative) GetAllTokens(ctx context.Context, in *protos.TokenReq) (*protos.TokenResp, error) {
	c, tkn, err := rn.getClient(ctx, true)
	if err != nil {
		return nil, err
	}
	tr, err := c.GetAllTokens(rn.c.GetTokenDID(tkn), in.TokenType)
	if err != nil {
		return nil, err
	}
	if !tr.Status {
		return nil, fmt.Errorf("failed to gt tokens, " + tr.Message)
	}
	resp := &protos.TokenResp{
		TokenDetials: make([]*protos.TokenDetial, 0),
	}
	for _, td := range tr.TokenDetails {
		t := &protos.TokenDetial{
			Token:      td.Token,
			TokenState: int32(td.Status),
		}
		resp.TokenDetials = append(resp.TokenDetials, t)
	}
	return resp, nil
}
