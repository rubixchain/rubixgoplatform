package core

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
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

// Add adds data to IPFS with health checks and retry logic.
// If provCtx is non-nil, a provider record is created after a successful add.
// Pass nil for OnlyHash or infrastructure calls that don't need tracking.
func (ops *IPFSOperations) Add(data io.Reader, provCtx *types.IPFSProviderContext, opts ...ipfsnode.AddOpts) (string, error) {
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

	if provCtx != nil && ops.core.ipfsProviderStore != nil {
		ops.core.ipfsProviderStore.RecordProvider(result, provCtx, constants.IPFSProviderOpAdd)
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

	inputHash := hash
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		inputHash, err = ops.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return nil, err
		}
	}

	metadata := map[string]interface{}{
		"hash": inputHash,
	}

	err := ops.executeWithMetrics(ctx, "ipfs.cat", metadata, func() error {
		reader, err := ops.core.ipfs.Cat(inputHash)
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	inputHash := hash
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		inputHash, err = ops.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}
	}

	metadata := map[string]interface{}{
		"hash": inputHash,
		"path": path,
	}

	return ops.executeWithMetrics(ctx, "ipfs.get", metadata, func() error {
		return ops.core.ipfs.Get(inputHash, path)
	})
}

// Pin pins a hash in IPFS with health checks.
// If provCtx is non-nil, a provider record is created after a successful pin.
func (ops *IPFSOperations) Pin(hash string, provCtx *types.IPFSProviderContext) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inputHash := hash
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		inputHash, err = ops.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}
	}

	metadata := map[string]interface{}{
		"hash": inputHash,
	}

	err := ops.executeWithMetrics(ctx, "ipfs.pin", metadata, func() error {
		return ops.core.ipfs.Pin(inputHash)
	})

	if err == nil && provCtx != nil && ops.core.ipfsProviderStore != nil {
		ops.core.ipfsProviderStore.RecordProvider(inputHash, provCtx, constants.IPFSProviderOpPin)
	}

	return err
}

// Unpin unpins a hash in IPFS with health checks.
// If provCtx is non-nil, a provider record is created and existing records are marked unpinned.
func (ops *IPFSOperations) Unpin(hash string, provCtx *types.IPFSProviderContext) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inputHash := hash
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		inputHash, err = ops.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}
	}

	metadata := map[string]interface{}{
		"hash": inputHash,
	}

	err := ops.executeWithMetrics(ctx, "ipfs.unpin", metadata, func() error {
		return ops.core.ipfs.Unpin(inputHash)
	})

	if err == nil && ops.core.ipfsProviderStore != nil {
		if provCtx != nil {
			ops.core.ipfsProviderStore.RecordProvider(inputHash, provCtx, constants.IPFSProviderOpUnpin)
		}
		ops.core.ipfsProviderStore.MarkUnpinned(inputHash)
	}

	return err
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
