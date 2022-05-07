package core

import (
	"context"
	"fmt"
	"net"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
)

func getPingAppName(prefix string) string {
	return prefix + "Ping"
}

func (c *Core) handlePing(conn net.Conn) {
	buff := make([]byte, 128)
	l, err := conn.Read(buff)
	if err != nil {
		conn.Close()
		return
	}
	if string(buff[:l]) == "PingCheck" {
		fmt.Printf("Ping received\n")
		conn.Write([]byte("PingResponse"))
		conn.Close()
	} else {
		fmt.Printf("Failed to recevie\n")
		conn.Close()
	}
}

func (c *Core) PingPeer(peerdID string) (string, error) {
	cfg := &ipfsport.Config{
		AppName: getPingAppName(peerdID),
		Listner: false,
		Port:    c.cfg.CfgData.Ports.ReceiverPort + 11,
		PeerID:  peerdID,
	}

	err := c.ipfs.SwarmConnect(context.Background(), "/ipfs/"+peerdID)
	if err != nil {
		c.log.Error("Failed to connect swarm peer", "err", err)
		return "", err
	}
	p, err := ipfsport.NewIPFSPort(cfg, c.ipfs, c.log, nil)
	if err != nil {
		return "", err
	}
	err = p.Start()
	if err != nil {
		return "", err
	}
	err = p.SendBytes([]byte("PingCheck"))
	if err != nil {
		return "", err
	}
	buff, err := p.ReadBhtes(30)
	if err != nil {
		return "", err
	}
	fmt.Printf("Received : %s\n", string(buff))
	return "", nil
}
