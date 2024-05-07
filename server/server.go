package server

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/grpcserver"
	"github.com/rubixchain/rubixgoplatform/setup"
	ccfg "github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// Server defines server handle
type Server struct {
	ensweb.Server
	cfg  *Config
	log  logger.Logger
	c    *core.Core
	sc   chan bool
	grpc *grpcserver.ServerGRPC
}

// NewServer create new server instances
func NewServer(c *core.Core, cfg *Config, log logger.Logger, start bool, sc chan bool, timeout time.Duration) (*Server, error) {
	s := &Server{cfg: cfg, sc: sc, c: c}
	var err error
	s.log = log.Named("Rubixplatform")
	if err != nil {
		s.log.Error("failed to create core", "err", err)
		return nil, err
	}
	if cfg.EnableAuth {
		if cfg.DBType == "" {
			cfg.DBType = "Sqlite3"
			cfg.DBAddress = "rubix.db"
		}
	}
	s.Server, err = ensweb.NewServer(&cfg.Config, nil, log, ensweb.SetServerTimeout(timeout))
	if err != nil {
		s.log.Error("failed to create server", "err", err)
		return nil, err
	}

	s.SetDebugMode()

	if cfg.EnableAuth {
		if cfg.APIKey == "" {
			cfg.APIKey = "TestAPIKey"
		}
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = BasicAuthMethod
		}
		if cfg.SessionName == "" {
			cfg.SessionName = "AuthSession"
		}
		if cfg.SessionKey == "" {
			rb := make([]byte, 32)
			rand.Read(rb)
			cfg.SessionKey = base64.StdEncoding.EncodeToString(rb)
		}
		s.CreateSessionStore(cfg.SessionName, "HaiHello", sessions.Options{Path: "/api", HttpOnly: true})
	}

	s.SetShutdown(s.ExitFunc)
	err = s.c.RunIPFS()
	if err != nil {
		s.c.StopCore()
		s.log.Error("failed to create server", "err", err)
		return nil, err
	}
	err = s.c.SetupCore()
	if err != nil {
		s.c.StopCore()
		s.log.Error("failed to create server", "err", err)
		return nil, err
	}
	//s.CreateSessionStore(sessionStore, cfg.ClientSecret, sessions.Options{})
	if start {
		ok, _ := s.c.Start()
		if !ok {
			s.log.Error("failed to start core")
			return nil, fmt.Errorf("failed to start core")
		}
	}
	cc := &ccfg.Config{
		ServerAddress: cfg.Config.HostAddress,
		ServerPort:    cfg.Config.HostPort,
	}
	s.grpc, err = grpcserver.NewServerGRPC(c, cc, log, cfg.GRPCAddr, cfg.GRPCSecure)
	if err != nil {
		s.log.Error("Failed to create GRPC server", "err", err)
		return nil, err
	}
	go s.grpc.Run()
	s.RegisterRoutes()
	return s, nil
}

// RegisterRoutes register all routes
func (s *Server) RegisterRoutes() {
	s.AddRoute("/", "GET", s.Index)
	s.AddRoute(setup.APIStart, "GET", s.AuthHandle(s.APIStart, false, s.AuthError, true))
	s.AddRoute(setup.APIShutdown, "POST", s.AuthHandle(s.APIShutdown, false, s.AuthError, true))
	s.AddRoute(setup.APINodeStatus, "GET", s.AuthHandle(s.APINodeStatus, false, s.AuthError, false))
	s.AddRoute(setup.APIPing, "GET", s.AuthHandle(s.APIPing, false, s.AuthError, false))
	s.AddRoute(setup.APIAddBootStrap, "POST", s.AuthHandle(s.APIAddBootStrap, false, s.AuthError, true))
	s.AddRoute(setup.APIRemoveBootStrap, "POST", s.AuthHandle(s.APIRemoveBootStrap, false, s.AuthError, true))
	s.AddRoute(setup.APIRemoveAllBootStrap, "POST", s.AuthHandle(s.APIRemoveAllBootStrap, false, s.AuthError, true))
	s.AddRoute(setup.APIGetAllBootStrap, "GET", s.AuthHandle(s.APIGetAllBootStrap, false, s.AuthError, true))
	s.AddRoute(setup.APIGetDIDChallenge, "GET", s.APIGetDIDChallenge)
	s.AddRoute(setup.APIGetDIDAccess, "POST", s.APIGetDIDAccess)
	s.AddRoute(setup.APICreateDID, "POST", s.APICreateDID)
	s.AddRoute(setup.APIGetAllTokens, "GET", s.AuthHandle(s.APIGetAllTokens, true, s.AuthError, false))
	s.AddRoute(setup.APIGetAllDID, "GET", s.AuthHandle(s.APIGetAllDID, true, s.AuthError, true))
	s.AddRoute(setup.APIAddQuorum, "POST", s.AuthHandle(s.APIAddQuorum, true, s.AuthError, true))
	s.AddRoute(setup.APIGetAllQuorum, "GET", s.AuthHandle(s.APIGetAllQuorum, true, s.AuthError, true))
	s.AddRoute(setup.APIRemoveAllQuorum, "GET", s.AuthHandle(s.APIRemoveAllQuorum, true, s.AuthError, true))
	s.AddRoute(setup.APISetupQuorum, "POST", s.AuthHandle(s.APISetupQuorum, true, s.AuthError, true))
	s.AddRoute(setup.APISetupService, "POST", s.AuthHandle(s.APISetupService, true, s.AuthError, true))
	s.AddRoute(setup.APIGenerateTestToken, "POST", s.AuthHandle(s.APIGenerateTestToken, true, s.AuthError, false))
	s.AddRoute(setup.APIInitiateRBTTransfer, "POST", s.AuthHandle(s.APIInitiateRBTTransfer, true, s.AuthError, false))
	s.AddRoute(setup.APIGetAccountInfo, "GET", s.AuthHandle(s.APIGetAccountInfo, true, s.AuthError, false))
	s.AddRoute(setup.APISignatureResponse, "POST", s.AuthHandle(s.APISignatureResponse, true, s.AuthError, false))
	s.AddRoute(setup.APIDumpTokenChainBlock, "POST", s.AuthHandle(s.APIDumpTokenChainBlock, true, s.AuthError, false))
	s.AddRoute(setup.APIRegisterDID, "POST", s.AuthHandle(s.APIRegisterDID, true, s.AuthError, false))
	s.AddRoute(setup.APISetupDID, "POST", s.AuthHandle(s.APISetupDID, true, s.AuthError, false))
	s.AddRoute(setup.APIMigrateNode, "POST", s.APIMigrateNode)
	s.AddRoute(setup.APILockTokens, "POST", s.AuthHandle(s.APILockTokens, true, s.AuthError, false))
	s.AddRoute(setup.APICreateDataToken, "POST", s.AuthHandle(s.APICreateDataToken, true, s.AuthError, false))
	s.AddRoute(setup.APICommitDataToken, "POST", s.AuthHandle(s.APICommitDataToken, true, s.AuthError, false))
	s.AddRoute(setup.APICheckDataToken, "POST", s.AuthHandle(s.APICheckDataToken, true, s.AuthError, false))
	s.AddRoute(setup.APIGetDataToken, "GET", s.AuthHandle(s.APIGetDataToken, true, s.AuthError, false))
	s.AddRoute(setup.APISetupDB, "POST", s.AuthHandle(s.APISetupDB, true, s.AuthError, true))
	s.AddRoute(setup.APIGetTxnByTxnID, "GET", s.AuthHandle(s.APIGetTxnByTxnID, true, s.AuthError, false))
	s.AddRoute(setup.APIGetTxnByDID, "GET", s.AuthHandle(s.APIGetTxnByDID, true, s.AuthError, false))
	s.AddRoute(setup.APIGetTxnByComment, "GET", s.AuthHandle(s.APIGetTxnByComment, true, s.AuthError, false))
	s.AddRoute(setup.APICreateNFT, "POST", s.AuthHandle(s.APICreateNFT, true, s.AuthError, false))
	s.AddRoute(setup.APIGetAllNFT, "GET", s.AuthHandle(s.APIGetAllNFT, true, s.AuthError, false))
	s.AddRoute(setup.APIAddNFTSale, "GET", s.AuthHandle(s.APIAddNFTSale, true, s.AuthError, false))
	s.AddRoute(setup.APIDeploySmartContract, "POST", s.AuthHandle(s.APIDeploySmartContract, true, s.AuthError, false))
	s.AddRoute(setup.APIGenerateSmartContract, "POST", s.AuthHandle(s.APIGenerateSmartContract, true, s.AuthError, false))
	s.AddRoute(setup.APIFetchSmartContract, "POST", s.AuthHandle(s.APIFetchSmartContract, true, s.AuthError, false))
	s.AddRoute(setup.APIPublishContract, "POST", s.AuthHandle(s.APIPublishContract, true, s.AuthError, false))
	s.AddRoute(setup.APISubscribecontract, "POST", s.AuthHandle(s.APISubscribecontract, true, s.AuthError, false))
	s.AddRoute(setup.APIDumpSmartContractTokenChainBlock, "POST", s.AuthHandle(s.APIDumpSmartContractTokenChainBlock, true, s.AuthError, false))
	s.AddRoute(setup.APIExecuteSmartContract, "POST", s.AuthHandle(s.APIExecuteSmartContract, true, s.AuthError, false))
	s.AddRoute(setup.APIGetSmartContractTokenData, "POST", s.AuthHandle(s.APIGetSmartContractTokenChainData, true, s.AuthError, false))
	s.AddRoute(setup.APIRegisterCallBackURL, "POST", s.AuthHandle(s.APIRegisterCallbackURL, true, s.AuthError, false))
	s.AddRoute(setup.APIGetTxnByNode, "GET", s.AuthHandle(s.APIGetTxnByNode, true, s.AuthError, false))
	s.AddRoute(setup.APIRemoveTokenChainBlock, "POST", s.AuthHandle(s.APIRemoveTokenChainBlock, true, s.AuthError, false))
	s.AddRoute(setup.APIPeerID, "GET", s.AuthHandle(s.APIPeerID, false, s.AuthError, false))
	s.AddRoute(setup.APIReleaseAllLockedTokens, "GET", s.AuthHandle(s.APIReleaseAllLockedTokens, true, s.AuthError, false))
	s.AddRoute(setup.APICheckQuorumStatus, "GET", s.AuthHandle(s.APICheckQuorumStatus, false, s.AuthError, false))

}

func (s *Server) ExitFunc() error {
	s.c.StopCore()
	return nil
}

func (s *Server) Index(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSONError(req, http.StatusForbidden, InvalidRequestErr, InvalidRequestErr)
}

func (s *Server) AuthHandle(hf ensweb.HandlerFunc, did bool, ef ensweb.HandlerFunc, root bool) ensweb.HandlerFunc {
	if s.cfg.EnableAuth {
		switch s.cfg.AuthMethod {
		case BasicAuthMethod:
			if did {
				return s.DIDAuthHandle(hf, nil, ef, root)
			} else {
				return s.APIKeyAuthHandle(hf, ef)
			}
		// case SessionAuthMethod:
		// 	return s.SessionAuthHandle(&setup.BearerToken{}, s.cfg.SessionName, s.cfg.SessionKey, hf, ef)
		// case BasicAuthMethod:
		// 	return s.BasicAuthHandle(&Token{}, hf)
		default:
			return ensweb.HandlerFunc(func(req *ensweb.Request) *ensweb.Result {
				if ef == nil {
					return s.RenderJSONError(req, http.StatusForbidden, "invalid session", "invalid session")
				} else {
					return ef(req)
				}
			})
		}
	} else {
		return ensweb.HandlerFunc(func(req *ensweb.Request) *ensweb.Result {
			return hf(req)
		})
	}
}
