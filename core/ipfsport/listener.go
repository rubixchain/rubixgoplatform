package ipfsport

import (
	"context"
	"fmt"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	srvcfg "github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

type Listener struct {
	ensweb.Server
	cfg  *Config
	ipfs *ipfsnode.Shell
	log  logger.Logger
}

func NewListener(cfg *Config, log logger.Logger, ipfs *ipfsnode.Shell) (*Listener, error) {
	l := &Listener{
		cfg:  cfg,
		log:  log,
		ipfs: ipfs,
	}
	scfg := &srvcfg.Config{
		HostAddress: "localhost",
		HostPort:    fmt.Sprintf("%d", cfg.Port),
	}
	var err error
	l.Server, err = ensweb.NewServer(scfg, nil, log, ensweb.SetServerTimeout(time.Minute*10))
	if err != nil {
		l.log.Error("failed to create ensweb server", "err", err)
		return nil, err
	}
	err = l.listenIPFSPort()
	if err != nil {
		l.log.Error("failed to create ipfs listener", "err", err)
		return nil, err
	}
	l.SetShutdown(l.ExitFunc)
	return l, nil
}

func (l *Listener) listenIPFSPort() error {
	proto := "/x/" + l.cfg.AppName + "/1.0"
	addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", l.cfg.Port)
	resp, err := l.ipfs.Request("p2p/listen", proto, addr).Send(context.Background())
	if err != nil {
		return err
	}
	defer resp.Close()
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (l *Listener) ExitFunc() error {
	return nil
}
