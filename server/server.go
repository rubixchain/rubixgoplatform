package server

import (
	"fmt"
	"net/http"
	"time"

	srvcfg "github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core"
)

const (
	APIStart               string = "/api/start"
	APIShutdown            string = "/api/shutdown"
	APIPing                string = "/api/ping"
	APICreateDID           string = "/api/create"
	APIAddBootStrap        string = "/api/add-bootstrap"
	APIRemoveBootStrap     string = "/api/remove-bootstrap"
	APIRemoveAllBootStrap  string = "/api/remove-all-bootstrap"
	APIGetAllBootStrap     string = "/api/get-all-bootstrap"
	APIEnableExplorer      string = "/api/enable-explorer"
	APIInitiateRBTTransfer string = "/api/initiate-rbt-transfer"
)

// Server defines server handle
type Server struct {
	ensweb.Server
	log logger.Logger
	c   *core.Core
	sc  chan bool
}

// NewServer create new server instances
func NewServer(c *core.Core, scfg *srvcfg.Config, log logger.Logger, start bool, sc chan bool) (*Server, error) {
	s := &Server{sc: sc, c: c}
	var err error
	s.log = log.Named("Rubixplatform")
	if err != nil {
		s.log.Error("failed to create core", "err", err)
		return nil, err
	}
	s.Server, err = ensweb.NewServer(scfg, nil, log, ensweb.SetServerTimeout(time.Minute*10))
	if err != nil {
		s.log.Error("failed to create server", "err", err)
		return nil, err
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
	s.AddRoute(APIStart, "GET", s.APIStart)
	s.AddRoute(APIShutdown, "POST", s.APIShutdown)
	s.AddRoute(APIPing, "GET", s.APIPing)
	s.AddRoute(APIAddBootStrap, "POST", s.APIAddBootStrap)
	s.AddRoute(APIRemoveBootStrap, "POST", s.APIRemoveBootStrap)
	s.AddRoute(APIRemoveAllBootStrap, "POST", s.APIRemoveAllBootStrap)
	s.AddRoute(APIGetAllBootStrap, "GET", s.APIGetAllBootStrap)
	s.AddRoute(APICreateDID, "POST", s.APICreateDID)
	s.AddRoute(APIEnableExplorer, "POST", s.APIEnableExplorer)
	s.AddRoute(APIInitiateRBTTransfer, "POST", s.APIInitiateRBTTransfer)
}

func (s *Server) ExitFunc() error {
	s.c.StopCore()
	return nil
}

func (s *Server) Index(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSONError(req, http.StatusForbidden, InvalidRequestErr, InvalidRequestErr)
}
