package server

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/logger"
	"github.com/gorilla/sessions"
	"github.com/rubixchain/rubixgoplatform/core"
)

const (
	APILogin               string = "/api/login"
	APIStart               string = "/api/start"
	APIShutdown            string = "/api/shutdown"
	APIPing                string = "/api/ping"
	APIAddBootStrap        string = "/api/add-bootstrap"
	APIRemoveBootStrap     string = "/api/remove-bootstrap"
	APIRemoveAllBootStrap  string = "/api/remove-all-bootstrap"
	APIGetAllBootStrap     string = "/api/get-all-bootstrap"
	APICreateDID           string = "/api/createdid"
	APIGetAllDID           string = "/api/getalldid"
	APIAddQuorum           string = "/api/addquorum"
	APIGetAllQuorum        string = "/api/getallquorum"
	APIRemoveAllQuorum     string = "/api/removeallquorum"
	APISetupQuorum         string = "/api/setup-quorum"
	APIEnableExplorer      string = "/api/enable-explorer"
	APIGenerateTestToken   string = "/api/generate-test-token"
	APIInitiateRBTTransfer string = "/api/initiate-rbt-transfer"
	APIGetAccountInfo      string = "/api/get-account-info"
	APISignatureResponse   string = "/api/signature-response"
)

// Server defines server handle
type Server struct {
	ensweb.Server
	cfg *Config
	log logger.Logger
	c   *core.Core
	sc  chan bool
}

// NewServer create new server instances
func NewServer(c *core.Core, cfg *Config, log logger.Logger, start bool, sc chan bool) (*Server, error) {
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
	s.Server, err = ensweb.NewServer(&cfg.Config, nil, log, ensweb.SetServerTimeout(time.Minute*10))
	if err != nil {
		s.log.Error("failed to create server", "err", err)
		return nil, err
	}

	s.SetDebugMode()

	if cfg.EnableAuth {
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = SessionAuthMethod
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
		ecfg := ensweb.EntityConfig{
			DefaultTenantName:    "root",
			DefaultAdminName:     "admin",
			DefaultAdminPassword: "admin@123",
			TenantTableName:      "tenanttable",
			UserTableName:        "usertable",
			RoleTableName:        "roletable",
			UserRoleTableName:    "userroletable",
		}
		err = s.SetupEntity(ecfg)
		if err != nil {
			s.log.Error("failed to create entity", "err", err)
			return nil, err
		}
		t, err := s.GetTenant(ecfg.DefaultTenantName)
		if err != nil {
			s.log.Error("failed to get default tenant", "err", err)
			return nil, err
		}
		r, err := s.GetRole("admin")
		if err != nil {
			r = &ensweb.Role{
				TenantID:       t.ID,
				Name:           "admin",
				NormalizedName: strings.ToUpper("admin"),
			}
			err = s.CreateRole(r)
			if err != nil {
				s.log.Error("failed to get create role", "err", err)
				return nil, err
			}
		}
		r, err = s.GetRole("user")
		if err != nil {
			r = &ensweb.Role{
				TenantID:       t.ID,
				Name:           "user",
				NormalizedName: strings.ToUpper("user"),
				IsDefault:      true,
			}
			err = s.CreateRole(r)
			if err != nil {
				s.log.Error("failed to create role", "err", err)
				return nil, err
			}
		}
		u, err := s.GetUser(t.ID, ecfg.DefaultAdminName)
		if err != nil {
			u = &ensweb.User{
				Base: ensweb.Base{
					TenantID:             t.ID,
					CreationTime:         time.Now(),
					LastModificationTime: time.Now(),
				},
				Name:               "Administrator",
				UserName:           ecfg.DefaultAdminName,
				NormalizedUserName: strings.ToUpper(ecfg.DefaultAdminName),
				Roles: []ensweb.Role{
					{
						Name: "admin",
					},
				},
			}
			err = s.CreateUser(u)
			if err != nil {
				s.log.Error("failed to create user", "err", err)
				return nil, err
			}
		}
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
	s.RegisterRoutes()
	return s, nil
}

// RegisterRoutes register all routes
func (s *Server) RegisterRoutes() {
	s.AddRoute("/", "GET", s.Index)
	s.AddRoute(APILogin, "POST", s.APILogin)
	s.AddRoute(APIStart, "GET", s.AuthHandle(s.APIStart, s.ErrorFunc))
	s.AddRoute(APIShutdown, "POST", s.AuthHandle(s.APIShutdown, s.ErrorFunc))
	s.AddRoute(APIPing, "GET", s.AuthHandle(s.APIPing, s.ErrorFunc))
	s.AddRoute(APIAddBootStrap, "POST", s.AuthHandle(s.APIAddBootStrap, s.ErrorFunc))
	s.AddRoute(APIRemoveBootStrap, "POST", s.AuthHandle(s.APIRemoveBootStrap, s.ErrorFunc))
	s.AddRoute(APIRemoveAllBootStrap, "POST", s.AuthHandle(s.APIRemoveAllBootStrap, s.ErrorFunc))
	s.AddRoute(APIGetAllBootStrap, "GET", s.AuthHandle(s.APIGetAllBootStrap, s.ErrorFunc))
	s.AddRoute(APICreateDID, "POST", s.AuthHandle(s.APICreateDID, s.ErrorFunc))
	s.AddRoute(APIGetAllDID, "GET", s.AuthHandle(s.APIGetAllDID, s.ErrorFunc))
	s.AddRoute(APIAddQuorum, "POST", s.AuthHandle(s.APIAddQuorum, s.ErrorFunc))
	s.AddRoute(APIGetAllQuorum, "GET", s.AuthHandle(s.APIGetAllQuorum, s.ErrorFunc))
	s.AddRoute(APIRemoveAllQuorum, "GET", s.AuthHandle(s.APIRemoveAllQuorum, s.ErrorFunc))
	s.AddRoute(APISetupQuorum, "POST", s.AuthHandle(s.APISetupQuorum, s.ErrorFunc))
	s.AddRoute(APIEnableExplorer, "POST", s.AuthHandle(s.APIEnableExplorer, s.ErrorFunc))
	s.AddRoute(APIGenerateTestToken, "POST", s.AuthHandle(s.APIGenerateTestToken, s.ErrorFunc))
	s.AddRoute(APIInitiateRBTTransfer, "POST", s.AuthHandle(s.APIInitiateRBTTransfer, s.ErrorFunc))
	s.AddRoute(APIGetAccountInfo, "GET", s.AuthHandle(s.APIGetAccountInfo, s.ErrorFunc))
	s.AddRoute(APISignatureResponse, "POST", s.AuthHandle(s.APISignatureResponse, s.ErrorFunc))

}

func (s *Server) ExitFunc() error {
	s.c.StopCore()
	return nil
}

func (s *Server) ErrorFunc(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSONError(req, http.StatusForbidden, InvalidRequestErr, InvalidRequestErr)
}

func (s *Server) Index(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSONError(req, http.StatusForbidden, InvalidRequestErr, InvalidRequestErr)
}

func (s *Server) AuthHandle(hf ensweb.HandlerFunc, ef ensweb.HandlerFunc) ensweb.HandlerFunc {
	if s.cfg.EnableAuth {
		switch s.cfg.AuthMethod {
		case SessionAuthMethod:
			return s.SessionAuthHandle(&Token{}, s.cfg.SessionName, s.cfg.SessionKey, hf, ef)
		default:
			return ensweb.HandlerFunc(func(req *ensweb.Request) *ensweb.Result {
				if ef == nil {
					return s.RenderJSONError(req, http.StatusForbidden, "invalid session", "invalid session")
				} else {
					return hf(req)
				}
			})
		}
	} else {
		return ensweb.HandlerFunc(func(req *ensweb.Request) *ensweb.Result {
			return hf(req)
		})
	}
}
