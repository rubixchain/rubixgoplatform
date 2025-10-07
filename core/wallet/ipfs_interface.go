package wallet

import (
	"io"

	ipfsnode "github.com/ipfs/go-ipfs-api"
)

// IPFSOperations defines the interface for IPFS operations
// This allows the wallet to use either direct IPFS or health-managed operations
type IPFSOperations interface {
	Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error)
	Cat(hash string) (io.ReadCloser, error)
	Get(hash, path string) error
	Pin(hash string) error
	Unpin(hash string) error
}

// DirectIPFSOperations wraps the IPFS shell for direct operations
type DirectIPFSOperations struct {
	ipfs *ipfsnode.Shell
}

// NewDirectIPFSOperations creates a new direct IPFS operations wrapper
func NewDirectIPFSOperations(ipfs *ipfsnode.Shell) *DirectIPFSOperations {
	return &DirectIPFSOperations{ipfs: ipfs}
}

func (d *DirectIPFSOperations) Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error) {
	return d.ipfs.Add(data, opts...)
}

func (d *DirectIPFSOperations) Cat(hash string) (io.ReadCloser, error) {
	return d.ipfs.Cat(hash)
}

func (d *DirectIPFSOperations) Get(hash, path string) error {
	return d.ipfs.Get(hash, path)
}

func (d *DirectIPFSOperations) Pin(hash string) error {
	return d.ipfs.Pin(hash)
}

func (d *DirectIPFSOperations) Unpin(hash string) error {
	return d.ipfs.Unpin(hash)
}