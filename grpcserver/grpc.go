package grpcserver

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/protos"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	ChanllegeTokenType string = "challengeToken"
	AccessTokenType    string = "accessToken"
)

const (
	Issuer string = "Fexr Sky"
)

type RubixNative struct {
	protos.UnimplementedRubixServiceServer
	tokenSecret  string
	randomSecret string
	c            *core.Core
	cfg          *config.Config
	log          logger.Logger
}

type ServerGRPC struct {
	c      *core.Core
	log    logger.Logger
	addr   string
	secure bool
	Native *RubixNative
}

func ValidateToken(ctx context.Context, claims jwt.Claims, secret string) (bool, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false, false
	}
	authHeader, ok := md["authorization"]
	if !ok {
		return false, false
	}
	tokenString := authHeader[0]
	return ValidateJWTToken(tokenString, claims, secret)
}

func getAuthToken(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	authHeader, ok := md["authorization"]
	if !ok {
		return "", false
	}
	tk, _ := strings.CutPrefix(authHeader[0], "Bearer ")
	return tk, true
}

func ValidateJWTToken(tokenString string, claims jwt.Claims, secret string) (bool, bool) {
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})
	return true, err != nil
}

func GenerateJWTToken(claims jwt.Claims, secret string) string {
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return ""
	}
	return tokenString
}

func NewServerGRPC(c *core.Core, cfg *config.Config, log logger.Logger, addr string, secure bool) (*ServerGRPC, error) {
	// cl, err := client.NewClient(cfg, log.Named("grpcclient"), 10*time.Minute)
	// if err != nil {
	// 	return nil, err
	// }
	s := &ServerGRPC{
		c:      c,
		log:    log.Named("grpc"),
		addr:   addr,
		secure: secure,
		Native: &RubixNative{c: c, cfg: cfg, randomSecret: util.GetRandString(), log: log.Named("native_grpc")},
	}
	s.log.Info("GRPC Server created")
	return s, nil
}

func loadTLSCredentials() (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	serverCert, err := tls.LoadX509KeyPair("server-cert.pem", "server-key.pem")
	if err != nil {
		return nil, err
	}

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	return credentials.NewTLS(config), nil
}

func (s *ServerGRPC) Run() {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.log.Panic("failed to listen", "err", err)
	}
	var server *grpc.Server
	if s.secure {
		tlsCredentials, err := loadTLSCredentials()
		if err != nil {
			s.log.Panic("cannot load TLS credentials: ", err)
		}
		server = grpc.NewServer(grpc.Creds(tlsCredentials))
	} else {
		server = grpc.NewServer()
	}
	// Creates a new gRPC server with UnaryInterceptor

	protos.RegisterRubixServiceServer(server, s.Native)
	s.log.Info("Running GRPC server...")
	server.Serve(lis)
}

func (rn *RubixNative) getClient(ctx context.Context, auth bool) (*client.Client, string, error) {
	c, err := client.NewClient(rn.cfg, rn.log.Named("grpcclient"), 10*time.Minute)
	if err != nil {
		return nil, "", err
	}
	if !auth {
		return c, "", nil
	}
	tkn, ok := getAuthToken(ctx)
	if !ok {
		return nil, "", status.Errorf(codes.Unauthenticated, err.Error())
	}
	c.SetAuthToken(tkn)
	return c, tkn, nil
}

func (rn *RubixNative) basicResponse(br *model.BasicResponse) (*protos.BasicReponse, error) {
	resp := &protos.BasicReponse{
		Status:  br.Status,
		Message: br.Message,
	}
	if br.Result == nil {
		return resp, nil
	}
	jb, err := json.Marshal(br.Result)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	var sr did.SignReqData
	err = json.Unmarshal(jb, &sr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	resp.SignNeeded = true
	resp.SignRequest = &protos.SignRequest{
		ReqID:       sr.ID,
		Mode:        int32(sr.Mode),
		Hash:        sr.Hash,
		OnlyPrivKey: sr.OnlyPrivKey,
	}
	return resp, nil
}

func (rn *RubixNative) StreamSignature(stream protos.RubixService_StreamSignatureServer) error {
	for {
		ctx := stream.Context()
		sr, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		c, _, err := rn.getClient(ctx, true)
		if err != nil {
			return err
		}
		req := &did.SignRespData{
			ID:       sr.ReqID,
			Mode:     int(sr.Mode),
			Password: sr.Password,
			Signature: did.DIDSignature{
				Pixels:    sr.ImgSign,
				Signature: sr.PvtSign,
			},
		}
		br, err := c.SignatureResponse(req)
		if err != nil {
			return err
		}
		resp, err := rn.basicResponse(br)
		if err != nil {
			return err
		}
		err = stream.Send(resp)
		if err != nil {
			return err
		}
		if !resp.SignNeeded {
			return nil
		}
	}
}
