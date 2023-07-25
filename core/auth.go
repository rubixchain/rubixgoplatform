package core

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) validateJWTToken(tokenString string, claims jwt.Claims) bool {
	tk, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return c.secret, nil
	})
	if err != nil {
		c.log.Error("Failed to validate token", "err", err)
		return false
	}
	return tk.Valid
}

func (c *Core) GetTokenDID(token string) string {
	var bt setup.BearerToken
	if !c.validateJWTToken(token, &bt) {
		return ""
	}
	if bt.PeerID != c.peerID {
		return ""
	}
	return bt.DID
}

func (c *Core) ValidateDIDToken(token string, tt string, did string) (*setup.BearerToken, bool) {
	var bt setup.BearerToken
	if !c.validateJWTToken(token, &bt) {
		return nil, false
	}
	if bt.PeerID != c.peerID {
		c.log.Error("Token not issued by the node", "node", bt.PeerID)
		return nil, false
	}
	if tt != bt.TokenType {
		c.log.Error("Invalid token type", "exp_type", tt, "rcv_type", bt.TokenType)
		return nil, false
	}
	if did != "" {
		if bt.DID != did {
			c.log.Error("Token is not belong to the DID", "exp_did", did, "rcv_did", bt.DID)
			return nil, false
		}
	}
	return &bt, true
}

func (c *Core) generateJWTToken(claims jwt.Claims) string {
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)
	tokenString, err := token.SignedString([]byte(c.secret))
	if err != nil {
		return ""
	}
	return tokenString
}

func (c *Core) generateDIDToken(tt string, did string, root bool, expiresAt time.Time) string {
	bt := &setup.BearerToken{
		TokenType: tt,
		DID:       did,
		PeerID:    c.peerID,
		Random:    util.GetRandString(),
		Root:      root,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.peerID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	return c.generateJWTToken(bt)
}
