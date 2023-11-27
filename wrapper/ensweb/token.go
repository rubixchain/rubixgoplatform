package ensweb

import (
	"fmt"

	"github.com/dgrijalva/jwt-go"
)

type TokenHelper interface {
	Path() string
	Erase() error
	Get() (string, error)
	Store(string) error
}

type TokenType uint8

const (
	// TokenTypeDefault means "use the default, if any, that is currently set
	// on the mount". If not set, results in a Service token.
	TokenTypeDefault TokenType = iota

	// TokenTypeService is a "normal" Vault token for long-lived services
	TokenTypeService

	// TokenTypeBatch is a batch token
	TokenTypeBatch
)

func (t TokenType) String() string {
	switch t {
	case TokenTypeDefault:
		return "default"
	case TokenTypeService:
		return "service"
	case TokenTypeBatch:
		return "batch"
	default:
		panic("unreachable")
	}
}

// GenerateJWTToken will generate JWT token
func (s *Server) GenerateJWTToken(claims jwt.Claims) string {
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)

	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return ""
	}
	return tokenString
}

// ValidateJWTToken verify token
func (s *Server) ValidateJWTToken(token string, claims jwt.Claims) error {
	_, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		//Make sure that the token method conform to "SigningMethodHMAC"
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	return err
}
