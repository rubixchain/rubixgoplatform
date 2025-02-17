package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

var jwtSecret = []byte("RubixBIPWallet")

// validateAccess : validate the access based on the client token,
// api key access will have rot directory access
func (s *Server) validateAccess(req *ensweb.Request) (string, bool) {
	if s.cfg.EnableAuth {
		if req.ClientToken.Verified {
			token := req.ClientToken.Model.(*setup.BearerToken)
			return token.DID, true
		} else if req.ClientToken.APIKeyVerified {
			return DIDRootDir, true
		} else {
			return "", false
		}
	} else {
		return DIDRootDir, true
	}
}

func (s *Server) AuthError(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "unauthorized access"}, http.StatusUnauthorized)
}

func (s *Server) DIDAuthHandle(hf ensweb.HandlerFunc, af ensweb.AuthFunc, ef ensweb.HandlerFunc, root bool) ensweb.HandlerFunc {
	return ensweb.HandlerFunc(func(req *ensweb.Request) *ensweb.Result {
		bt, ok := s.c.ValidateDIDToken(req.ClientToken.Token, setup.AccessTokenType, "")
		if !ok {
			if ef != nil {
				return ef(req)
			} else {
				return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "ivnalid token"}, http.StatusUnauthorized)
			}
		}
		if root && !bt.Root {
			return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "root access denied"}, http.StatusUnauthorized)
		}
		req.ClientToken.Model = bt
		req.ClientToken.Verified = true
		if af != nil {
			if !af(req) {
				if ef != nil {
					return ef(req)
				} else {
					return s.RenderJSONError(req, http.StatusForbidden, "Access denined", "Access denied")
				}
			}
		}
		return hf(req)
	})
}

func (s *Server) APIAuthenticateWalletJWT(req *ensweb.Request) *ensweb.Result {
	jwtHeader := req.Headers.Get("Authorization")
	if jwtHeader == "" {
		return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "ivnalid token"}, http.StatusUnauthorized)
	}

	// Strip "Bearer " prefix from the token
	const bearerPrefix = "Bearer "
	if len(jwtHeader) > len(bearerPrefix) && jwtHeader[:len(bearerPrefix)] == bearerPrefix {
		jwtHeader = jwtHeader[len(bearerPrefix):]
	} else {
		return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "Invalid token format"}, http.StatusUnauthorized)
	}

	// Verify jwt token
	token, err := jwt.Parse(jwtHeader, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		log.Println("JWT verification failed:", err)
	}

	// Extract and validate claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		log.Printf("Token authenticated. Claims: %v\n", claims)
	} else {
		return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "ivnalid token"}, http.StatusUnauthorized)
	}

	//proceed for transaction
	txnRequest := model.RBTTransferRequest{
		Receiver:   token.Claims.(jwt.MapClaims)["receiver_did"].(string),
		Sender:     token.Claims.(jwt.MapClaims)["did"].(string),
		TokenCount: token.Claims.(jwt.MapClaims)["rbt_amount"].(float64),
	}

	if token.Claims.(jwt.MapClaims)["comment"] != nil {
		txnRequest.Comment = token.Claims.(jwt.MapClaims)["comment"].(string)
	}
	if token.Claims.(jwt.MapClaims)["quorum_type"] != nil {
		txnRequest.Type = int(token.Claims.(jwt.MapClaims)["quorum_type"].(float64))
	} else {
		txnRequest.Type = 2
	}
	if token.Claims.(jwt.MapClaims)["password"] != nil {
		txnRequest.Password = token.Claims.(jwt.MapClaims)["password"].(string)
	} else {
		txnRequest.Password = "mypassword"
	}

	return s.TxnReqFromWallet(&txnRequest, req)
}
