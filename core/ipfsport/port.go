package ipfsport

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/EnsurityTechnologies/logger"
	ipfsnode "github.com/ipfs/go-ipfs-api"
)

type PortHandleFunc func(c net.Conn)

// Config will Port configuration
type Config struct {
	AppName string
	Listner bool
	Port    uint16
	PeerID  string
}

// Port holds Port handle
type IPFSPort struct {
	cfg        *Config
	ipfs       *ipfsnode.Shell
	log        logger.Logger
	l          net.Listener
	c          net.Conn
	lock       sync.Mutex
	started    bool
	handleFunc PortHandleFunc
	close      chan bool
}

// NewIPFSPort will create new Port
func NewIPFSPort(cfg *Config, ipfs *ipfsnode.Shell, log logger.Logger, handleFunc PortHandleFunc) (*IPFSPort, error) {
	p := &IPFSPort{
		cfg:        cfg,
		ipfs:       ipfs,
		log:        log,
		handleFunc: handleFunc,
	}
	p.close = make(chan bool, 1)
	return p, nil
}

// GetStatus will Port start status
func (p *IPFSPort) GetStatus() bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.started
}

// GetStatus will Port start status
func (p *IPFSPort) SetStatus(status bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.started = status
}

func (p *IPFSPort) listenIPFSPort() error {
	proto := "/x/" + p.cfg.AppName + "/1.0"
	addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", p.cfg.Port)
	resp, err := p.ipfs.Request("p2p/listen", proto, addr).Send(context.Background())
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (p *IPFSPort) forwardIPFSPort() error {
	proto := "/x/" + p.cfg.AppName + "/1.0"
	addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", p.cfg.Port)
	peer := "/p2p/" + p.cfg.PeerID
	resp, err := p.ipfs.Request("p2p/forward", proto, addr, peer).Send(context.Background())
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

// Start will start the Port listening port
func (p *IPFSPort) Start() error {
	if p.GetStatus() {
		return nil
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.cfg.Listner {
		err := p.listenIPFSPort()
		if err != nil {
			return err
		}
		l, err := net.Listen("tcp", "localhost"+":"+fmt.Sprintf("%d", p.cfg.Port))
		if err != nil {
			return err
		}
		fmt.Println("Port Listening")
		p.l = l
		p.started = true
		go p.accept()
	} else {
		err := p.forwardIPFSPort()
		if err != nil {
			return err
		}
		c, err := net.Dial("tcp", "localhost"+":"+fmt.Sprintf("%d", p.cfg.Port))
		if err != nil {
			return err
		}
		p.c = c
		p.started = true
	}
	return nil
}

func (p *IPFSPort) Stop() error {
	if !p.GetStatus() {
		return nil
	}
	if p.cfg.Listner {
		p.close <- true
		err := p.l.Close()
		if err != nil {
			<-p.close
			return err
		}
	} else {
		p.SetStatus(false)
		err := p.c.Close()
		return err
	}
	return nil
}

func (p *IPFSPort) accept() {
	for {
		c, err := p.l.Accept()
		fmt.Println("Socket Connected")
		if err != nil {
			select {
			case <-p.close:
				p.lock.Lock()
				p.started = false
				p.lock.Unlock()
			default:
				p.log.Error("Failed to accept connection", "err", err)
				p.lock.Lock()
				p.started = false
				p.lock.Unlock()
			}
			return
		}
		if p.handleFunc != nil {
			go p.handleFunc(c)
		}
	}
}

func (p *IPFSPort) SendBytes(buff []byte) error {
	if p.c == nil {
		return fmt.Errorf("no connection exist")
	}
	_, err := p.c.Write(buff)
	return err
}

func (p *IPFSPort) ReadBhtes(numBytes int) ([]byte, error) {
	if p.c == nil {
		return nil, fmt.Errorf("no connection exist")
	}
	buff := make([]byte, numBytes)
	l, err := p.c.Read(buff)
	if err != nil {
		return nil, err
	}
	return buff[:l], err
}
