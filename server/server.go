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
	APIStart string = "/api/start"
	APIPing  string = "/api/ping"
)

// Server defines server handle
type Server struct {
	ensweb.Server
	log logger.Logger
	c   *core.Core
}

// NewServer create new server instances
func NewServer(cfg *config.Config, log logger.Logger) (*Server, error) {
	s := &Server{}
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
	s.RegisterRoutes()

	return s, nil
}

// RegisterRoutes register all routes
func (s *Server) RegisterRoutes() {
	s.AddRoute("/", "GET", s.Index)
	s.AddRoute(APIStart, "GET", s.APIStart)
	s.AddRoute(APIPing, "GET", s.APIPing)
	fmt.Println(APIStart)
}

func (s *Server) ExitFunc() error {
	s.c.StopCore()
	return nil
}

func (s *Server) Index(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSONError(req, http.StatusForbidden, InvalidRequestErr, InvalidRequestErr)
}
