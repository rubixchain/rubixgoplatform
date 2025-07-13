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

// Add adds data to IPFS with health checks and retry logic
func (ops *IPFSOperations) Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error) {
	var result string
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
		hash, err := ops.core.ipfs.Add(data, opts...)
		if err != nil {
			operationErr = err
			return err
		}
		result = hash
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	err := ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
		hash, err := ops.core.ipfs.AddDir(path)
		if err != nil {
			operationErr = err
			return err
		}
		result = hash
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

	err := ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
		return ops.core.ipfs.Get(hash, path)
	})
}

// Pin pins a hash in IPFS with health checks
func (ops *IPFSOperations) Pin(hash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	return ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
		return ops.core.ipfs.Pin(hash)
	})
}

// Unpin unpins a hash in IPFS with health checks
func (ops *IPFSOperations) Unpin(hash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	return ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
		return ops.core.ipfs.Unpin(hash)
	})
}

// ID gets the IPFS node ID with health checks
func (ops *IPFSOperations) ID() (*ipfsnode.IdOutput, error) {
	var result *ipfsnode.IdOutput
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
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

	err := ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
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

	err := ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
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
	return ops.core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
		return ops.core.ipfs.SwarmConnect(ctx, addr)
	})
}

// Request makes an IPFS API request with health checks
func (ops *IPFSOperations) Request(command string, args ...string) *ipfsnode.RequestBuilder {
	return ops.core.ipfs.Request(command, args...)
}
