package servergrpc

import (
	"context"
	"fmt"
	"net"

	"github.com/EnsurityTechnologies/logger"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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
	tokenSecret string
	c           *core.Core
	log         logger.Logger
}

type ServerGRPC struct {
	c      *core.Core
	log    logger.Logger
	port   uint16
	Native *RubixNative
}

type BearerToken struct {
	TokenType string `json:"type"`
	PublicKey string `json:"publicKey"`
	PeerID    string `json:"peerId"`
	jwt.RegisteredClaims
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

func NewServerGRPC(c *core.Core, log logger.Logger, port uint16) (*ServerGRPC, error) {
	s := &ServerGRPC{
		c:      c,
		log:    log.Named("grpc"),
		port:   port,
		Native: &RubixNative{c: c, log: log.Named("native_grpc")},
	}
	s.log.Info("GRPC Server created")
	return s, nil
}

func (s *ServerGRPC) Run() {
	lis, err := net.Listen("tcp", fmt.Sprintf("%d", s.port))
	if err != nil {
		s.log.Panic("failed to listen", "err", err)
	}
	// Creates a new gRPC server with UnaryInterceptor
	server := grpc.NewServer()
	protos.RegisterRubixServiceServer(server, s.Native)
	s.log.Info("Running GRPC server...")
	server.Serve(lis)
}
