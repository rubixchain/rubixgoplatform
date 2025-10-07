package core

import (
	"fmt"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
)

// SendRequestWithReconnect wraps peer requests with automatic reconnection on failure
func (c *Core) SendRequestWithReconnect(
	p *ipfsport.Peer,
	peerID string,
	did string,
	method string,
	path string,
	query map[string]string,
	req interface{},
	resp interface{},
	includeDID bool,
	timeout ...time.Duration,
) error {
	// Register the connection if not already registered
	if c.p2pReconnect != nil {
		c.p2pReconnect.RegisterConnection(peerID, did, p)
	}
	
	// Try the request
	err := p.SendJSONRequest(method, path, query, req, resp, includeDID, timeout...)
	
	// If no error or no reconnect manager, return as is
	if err == nil || c.p2pReconnect == nil {
		return err
	}
	
	// Check if this is a connection error that we can recover from
	if c.p2pReconnect.IsConnectionError(err) {
		c.log.Warn("P2P connection error detected, attempting reconnection", 
			"peerID", peerID,
			"error", err)
		
		// Attempt to reconnect and get new peer connection
		newPeer, reconnectErr := c.p2pReconnect.HandleConnectionError(peerID, err)
		if reconnectErr != nil {
			c.log.Error("Failed to reconnect peer", 
				"peerID", peerID,
				"error", reconnectErr)
			return fmt.Errorf("connection lost and reconnection failed: %w", reconnectErr)
		}
		
		// Retry the request with the new connection
		// IMPORTANT: The 'req' parameter contains all the original signatures
		// from the sender. We're just retrying the same signed request over
		// a new p2p connection. No new signatures are required.
		c.log.Info("Retrying request with reconnected peer", "peerID", peerID)
		return newPeer.SendJSONRequest(method, path, query, req, resp, includeDID, timeout...)
	}
	
	// Not a connection error, return original error
	return err
}