package grpcserver

import (
	"context"

	"github.com/rubixchain/rubixgoplatform/protos"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (rn *RubixNative) GetBalance(ctx context.Context, in *emptypb.Empty) (*protos.GetBalanceRes, error) {
	c, tkn, err := rn.getClient(ctx, true)
	if err != nil {
		return nil, err
	}
	info, err := c.GetAccountInfo(rn.c.GetTokenDID(tkn))
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if info == nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	if !info.Status {
		return nil, status.Errorf(codes.Internal, info.Message)
	}
	return &protos.GetBalanceRes{Balance: info.AccountInfo[0].RBTAmount}, nil
}
