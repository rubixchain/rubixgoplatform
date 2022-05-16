package server

import (
	"fmt"
	"net/http"
	"time"

	srvcfg "github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
)

const (
	APIStart             string = "/api/start"
	APIShutdown          string = "/api/shutdown"
	APIPing              string = "/api/ping"
	APICreateDID         string = "/api/create"
	APISubscribeExplorer string = "/api/subscribe/explorer"
	APIPublishExplorer   string = "/api/publish/explorer"
)

// Server defines server handle
type Server struct {
	ensweb.Server
	log logger.Logger
	c   *core.Core
	sc  chan bool
}

// NewServer create new server instances
func NewServer(cfg *config.Config, log logger.Logger, start bool, sc chan bool) (*Server, error) {
	s := &Server{sc: sc}
	var err error
	s.log = log.Named("Rubixplatform")
	s.c, err = core.NewCore(cfg, s.log)
	if err != nil {
		s.log.Error("failed to create core", "err", err)
		return nil, err
	}

	scfg := &srvcfg.Config{
		HostAddress: cfg.NodeAddress,
		HostPort:    cfg.NodePort,
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
	s.AddRoute(APICreateDID, "POST", s.APICreateDID)
	s.AddRoute(APISubscribeExplorer, "POST", s.APISubscribeExplorer)
	s.AddRoute(APIPublishExplorer, "POST", s.APIPublishExplorer)
	fmt.Println(APIStart)
}

func (s *Server) ExitFunc() error {
	s.c.StopCore()
	return nil
}

func (s *Server) Index(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSONError(req, http.StatusForbidden, InvalidRequestErr, InvalidRequestErr)
}
