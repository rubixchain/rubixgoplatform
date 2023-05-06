package servergrpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/EnsurityTechnologies/enscrypt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/protos"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (rn *RubixNative) CreateDIDChallenge(ctx context.Context, req *protos.ChallengeReq) (*protos.ChallengeString, error) {
	expiresAt := time.Now().Add(time.Minute * 10)
	bt := BearerToken{
		TokenType: ChanllegeTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := GenerateJWTToken(&bt, rn.tokenSecret)
	return &protos.ChallengeString{Challenge: token}, nil
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
	token := req.EcdsaChallengeResponse.Payload
	var bt BearerToken
	ok, valid := ValidateJWTToken(token, &bt, rn.tokenSecret)
	if !ok || !valid {
		rn.log.Error("invalid token")
		return nil, status.Errorf(codes.Unauthenticated, "invalid token")
	}
	if !enscrypt.Verify(bt.Subject, []byte(token), req.EcdsaChallengeResponse.Signature) {
		rn.log.Error("invalid signature")
		return nil, status.Errorf(codes.Unauthenticated, "invalid signature")
	}

	folderName, err := rn.c.CreateTempFolder()
	if err != nil {
		rn.log.Error("failed to create folder")
		return nil, status.Errorf(codes.Internal, "failed to create folder")
	}
	defer os.RemoveAll(folderName)
	err = createFile(folderName+"/"+did.DIDImgFileName, req.DidImage, true)
	if err != nil {
		rn.log.Error(err.Error())
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	err = createFile(folderName+"/"+did.PubShareFileName, req.PublicShare, true)
	if err != nil {
		rn.log.Error(err.Error())
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	err = createFile(folderName+"/"+did.PubKeyFileName, req.PublicKey, false)
	if err != nil {
		rn.log.Error(err.Error())
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	didCreate := did.DIDCreate{
		Type:           did.WalletDIDMode,
		DIDImgFileName: folderName + "/" + did.DIDImgFileName,
		PubImgFile:     folderName + "/" + did.PubShareFileName,
		PubKeyFile:     folderName + "/" + did.PubKeyFileName,
		Dir:            "root",
	}
	did, err := rn.c.CreateDID(&didCreate)
	if err != nil {
		rn.log.Error("failed to create did", "err", err)
		return nil, status.Errorf(codes.Internal, "failed to create did, "+err.Error())
	}
	expiresAt := time.Now().Add(time.Hour * 24 * 30)
	bt = BearerToken{
		TokenType: AccessTokenType,
		PeerID:    rn.c.GetPeerID(),
		PublicKey: req.PublicKey,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   did,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token = GenerateJWTToken(&bt, rn.tokenSecret)
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
	return res, status.Errorf(codes.Unimplemented, "method CreateDID not implemented")
}

func (rn *RubixNative) GetAccessTokenChallenge(ctx context.Context, emt *emptypb.Empty) (*protos.ChallengeString, error) {
	var bt BearerToken
	// no need to validate the token
	ok, _ := ValidateToken(ctx, &bt, rn.tokenSecret)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "Authorization token is not supplied")
	}
	if bt.PublicKey == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Public key is missing")
	}
	expiresAt := time.Now().Add(time.Minute * 10)
	bt = BearerToken{
		TokenType: ChanllegeTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := GenerateJWTToken(&bt, rn.tokenSecret)
	return &protos.ChallengeString{Challenge: token}, nil
}
