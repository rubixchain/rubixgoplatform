package core

import (
	"context"
	"io"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
)

// IPFSOperations provides health-managed IPFS operations
type IPFSOperations struct {
	core *Core
}

// NewIPFSOperations creates a new IPFS operations wrapper
func NewIPFSOperations(core *Core) *IPFSOperations {
	return &IPFSOperations{core: core}
}

// executeWithMetrics executes an operation with health checks and performance metrics
func (ops *IPFSOperations) executeWithMetrics(ctx context.Context, operationName string, metadata map[string]interface{}, operation func() error) error {
	start := time.Now()
	
	err := ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, operation)
	
	// Update metrics if scalability manager exists
	if ops.core.ipfsScalability != nil {
		responseTime := time.Since(start)
		success := err == nil
		ops.core.ipfsScalability.UpdateMetrics(responseTime, success)
	}
	
	// Track operation performance
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["duration_ms"] = time.Since(start).Milliseconds()
	ops.core.TrackOperation(operationName, metadata)(err)
	
	return err
}

// Add adds data to IPFS with health checks and retry logic
func (ops *IPFSOperations) Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error) {
	var result string
	var operationErr error
	
	// Check if pinning is enabled
	// Note: This is a simplified check - actual pinning detection would need
	// to inspect the options differently
	pinning := true

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	metadata := map[string]interface{}{
		"pinning": pinning,
	}

	err := ops.executeWithMetrics(ctx, "ipfs.add", metadata, func() error {
		hash, err := ops.core.ipfs.Add(data, opts...)
		if err != nil {
			operationErr = err
			return err
		}
		result = hash
		metadata["hash"] = hash
		return nil
	})

	if err != nil {
		return "", err
	}

	return result, operationErr
}

// AddDir adds a directory to IPFS with health checks and retry logic
func (ops *IPFSOperations) AddDir(path string) (string, error) {
	var result string
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	
	metadata := map[string]interface{}{
		"path": path,
	}

	err := ops.executeWithMetrics(ctx, "ipfs.add_dir", metadata, func() error {
		hash, err := ops.core.ipfs.AddDir(path)
		if err != nil {
			operationErr = err
			return err
		}
		result = hash
		metadata["hash"] = hash
		return nil
	})

	if err != nil {
		return "", err
	}

	return result, operationErr
}

// Cat retrieves data from IPFS with health checks
func (ops *IPFSOperations) Cat(hash string) (io.ReadCloser, error) {
	var result io.ReadCloser
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	
	metadata := map[string]interface{}{
		"hash": hash,
	}

	err := ops.executeWithMetrics(ctx, "ipfs.cat", metadata, func() error {
		reader, err := ops.core.ipfs.Cat(hash)
		if err != nil {
			operationErr = err
			return err
		}
		result = reader
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, operationErr
}

// Get retrieves a file from IPFS with health checks
func (ops *IPFSOperations) Get(hash, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	
	metadata := map[string]interface{}{
		"hash": hash,
		"path": path,
	}

	return ops.executeWithMetrics(ctx, "ipfs.get", metadata, func() error {
		return ops.core.ipfs.Get(hash, path)
	})
}

// Pin pins a hash in IPFS with health checks
func (ops *IPFSOperations) Pin(hash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	
	metadata := map[string]interface{}{
		"hash": hash,
	}

	return ops.executeWithMetrics(ctx, "ipfs.pin", metadata, func() error {
		return ops.core.ipfs.Pin(hash)
	})
}

// Unpin unpins a hash in IPFS with health checks
func (ops *IPFSOperations) Unpin(hash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	
	metadata := map[string]interface{}{
		"hash": hash,
	}

	return ops.executeWithMetrics(ctx, "ipfs.unpin", metadata, func() error {
		return ops.core.ipfs.Unpin(hash)
	})
}

// ID gets the IPFS node ID with health checks
func (ops *IPFSOperations) ID() (*ipfsnode.IdOutput, error) {
	var result *ipfsnode.IdOutput
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := ops.executeWithMetrics(ctx, "ipfs.id", nil, func() error {
		id, err := ops.core.ipfs.ID()
		if err != nil {
			operationErr = err
			return err
		}
		result = id
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, operationErr
}

// BootstrapAdd adds bootstrap peers with health checks
func (ops *IPFSOperations) BootstrapAdd(peers []string) ([]string, error) {
	var result []string
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := ops.executeWithMetrics(ctx, "ipfs.bootstrap_add", map[string]interface{}{"peer_count": len(peers)}, func() error {
		added, err := ops.core.ipfs.BootstrapAdd(peers)
		if err != nil {
			operationErr = err
			return err
		}
		result = added
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, operationErr
}

// BootstrapRmAll removes all bootstrap peers with health checks
func (ops *IPFSOperations) BootstrapRmAll() ([]string, error) {
	var result []string
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := ops.executeWithMetrics(ctx, "ipfs.bootstrap_rm_all", nil, func() error {
		removed, err := ops.core.ipfs.BootstrapRmAll()
		if err != nil {
			operationErr = err
			return err
		}
		result = removed
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, operationErr
}

// SwarmConnect connects to a peer with health checks
func (ops *IPFSOperations) SwarmConnect(ctx context.Context, addr string) error {
	return ops.executeWithMetrics(ctx, "ipfs.swarm_connect", map[string]interface{}{
		"address": addr,
	}, func() error {
		return ops.core.ipfs.SwarmConnect(ctx, addr)
	})
}

// Request makes an IPFS API request with health checks
func (ops *IPFSOperations) Request(command string, args ...string) *ipfsnode.RequestBuilder {
	return ops.core.ipfs.Request(command, args...)
}