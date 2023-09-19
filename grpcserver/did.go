package grpcserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/protos"
	"github.com/rubixchain/rubixgoplatform/setup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (rn *RubixNative) GetDIDChallenge(ctx context.Context, in *protos.ChallengeReq) (*protos.ChallengeResp, error) {
	c, _, err := rn.getClient(ctx, false)
	if err != nil {
		return nil, err
	}
	token, err := c.GetDIDChallenge(in.Did)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return &protos.ChallengeResp{Challenge: token}, nil
}

func (rn *RubixNative) GetDIDAccess(ctx context.Context, in *protos.AccessReq) (*protos.Token, error) {
	c, _, err := rn.getClient(ctx, false)
	if err != nil {
		return nil, err
	}
	req := &model.GetDIDAccess{
		DID:      in.Did,
		Password: in.Password,
	}
	if in.Payload != nil {
		req.Token = in.Payload.Payload
		req.Signature = in.Payload.Signature
	}
	token, err := c.GetDIDAccess(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return &protos.Token{AccessToken: token}, nil
}

func createFile(fileName string, data string, decode bool) error {
	var ib []byte
	var err error
	if decode {
		ib, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			return fmt.Errorf("failed to decode base64 data")
		}
	} else {
		ib = []byte(data)
	}
	f, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create file")
	}
	f.Write(ib)
	f.Close()
	return nil
}

func (rn *RubixNative) CreateDID(ctx context.Context, req *protos.CreateDIDReq) (*protos.CreateDIDRes, error) {
	dc := &did.DIDCreate{
		Type:      int(req.DidMode),
		Secret:    req.Secret,
		MasterDID: req.MasterDid,
		PrivPWD:   req.PrivKeyPwd,
		QuorumPWD: req.QuorumKeyPwd,
	}
	folderName, err := rn.c.CreateTempFolder()
	if err != nil {
		rn.log.Error("failed to create folder")
		return nil, status.Errorf(codes.Internal, "failed to create folder")
	}
	defer os.RemoveAll(folderName)
	if req.DidImage != "" {
		err = createFile(folderName+"/"+did.DIDImgFileName, req.DidImage, true)
		if err != nil {
			rn.log.Error(err.Error())
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		dc.ImgFile = folderName + "/" + did.DIDImgFileName
	}
	if req.PublicShare != "" {
		err = createFile(folderName+"/"+did.PubShareFileName, req.PublicShare, true)
		if err != nil {
			rn.log.Error(err.Error())
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		dc.PubImgFile = folderName + "/" + did.PubShareFileName
	}
	if req.PublicKey != "" {
		err = createFile(folderName+"/"+did.PubKeyFileName, req.PublicKey, false)
		if err != nil {
			rn.log.Error(err.Error())
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		dc.PubKeyFile = folderName + "/" + did.PubKeyFileName
	}

	c, err := client.NewClient(rn.cfg, rn.log.Named("grpcclient"), 10*time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	did, ok := c.CreateDID(dc)
	if !ok {
		rn.log.Error("failed to create did")
		return nil, status.Errorf(codes.Internal, "failed to create did")
	}
	expiresAt := time.Now().Add(time.Hour * 24 * 30)
	bt := setup.BearerToken{
		TokenType: AccessTokenType,
		PeerID:    rn.c.GetPeerID(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   did,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := GenerateJWTToken(&bt, rn.tokenSecret)
	res := &protos.CreateDIDRes{
		Did:    did,
		Status: true,
		AccessToken: &protos.Token{
			AccessToken: token,
			Expiry: &timestamppb.Timestamp{
				Seconds: int64(expiresAt.Second()),
			},
		},
	}
	return res, nil
}
