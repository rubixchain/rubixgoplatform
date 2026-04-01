package grpcserver

import (
	"context"

	"github.com/rubixchain/rubixgoplatform/protos"
	"github.com/rubixchain/rubixgoplatform/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (rn *RubixNative) GetBalance(ctx context.Context, in *emptypb.Empty) (*protos.GetBalanceRes, error) {
	c, tkn, err := rn.getClient(ctx, true)
	if err != nil {
		return nil, err
	}
	info, err := c.GetRBTBalance(rn.c.GetTokenDID(tkn))
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if info == nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if !info.Status {
		return nil, status.Errorf(codes.Internal, info.Message)
	}
	rbtBalance := info.Result.(types.RBTBalance)
	return &protos.GetBalanceRes{Balance: rbtBalance.RBTBalance}, nil
}
