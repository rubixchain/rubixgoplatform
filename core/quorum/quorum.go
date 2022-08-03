package quorum

import (
	"bufio"
	"fmt"
	"net"
	"sync"

	"github.com/EnsurityTechnologies/logger"
)

type QuorumHandleFunc func(c net.Conn)

// Config will quorum configuration
type Config struct {
	Name     string `json:"name"`
	ConnType string `json:"conn_type"`
	Host     string `json:"host"`
	Port     string `json:"port"`
}

// Quorum holds quorum handle
type Quorum struct {
	cfg        *Config
	log        logger.Logger
	l          net.Listener
	lock       sync.Mutex
	started    bool
	handleFunc QuorumHandleFunc
	close      chan bool
}

// NewQuorum will create new quorum
func NewQuorum(cfg *Config, log logger.Logger, handleFunc QuorumHandleFunc) (*Quorum, error) {
	q := &Quorum{
		cfg:        cfg,
		log:        log,
		handleFunc: handleFunc,
	}
	q.close = make(chan bool, 1)
	return q, nil
}

// GetStatus will quorum start status
func (q *Quorum) GetStatus() bool {
	q.lock.Lock()
	defer q.lock.Unlock()
	return q.started
}

// Start will start the quorum listening port
func (q *Quorum) Start() error {
	if q.GetStatus() {
		return nil
	}
	q.lock.Lock()
	defer q.lock.Unlock()
	l, err := net.Listen(q.cfg.ConnType, q.cfg.Host+":"+q.cfg.Port)
	if err != nil {
		return err
	}
	fmt.Println("Quorum Listening")
	q.l = l
	q.started = true
	go q.accept()
	return nil
}

func (q *Quorum) Stop() error {
	if !q.GetStatus() {
		return nil
	}
	q.close <- true
	err := q.l.Close()
	if err != nil {
		<-q.close
		return err
	}
	return nil
}

func (q *Quorum) accept() {
	for {
		c, err := q.l.Accept()
		fmt.Println("Socket Connected")
		if err != nil {
			select {
			case <-q.close:
				q.lock.Lock()
				q.started = false
				q.lock.Unlock()
			default:
				q.log.Error("Failed to accept connection", "err", err)
				q.lock.Lock()
				q.started = false
				q.lock.Unlock()
			}
			return
		}
		if q.handleFunc != nil {
			go q.handleFunc(c)
		}
	}
}

func (q *Quorum) HandleConnection(c net.Conn) {
	for {
		buffer, err := bufio.NewReader(c).ReadBytes('\n')

		if err != nil {
			fmt.Println("Client left.")
			c.Close()
			return
		}

		str := string(buffer[:len(buffer)-1])

		fmt.Println("Client message:", str)

		str = "Server received your message : " + str + "\n"

		c.Write([]byte(str))
	}
}
