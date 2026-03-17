package types

import (
	"bytes"
	"io"
	"strings"

	ipfsnode "github.com/ipfs/go-ipfs-api"
)

type IPFSOperations interface {
	Add(data io.Reader, provCtx *IPFSProviderContext, opts ...ipfsnode.AddOpts) (string, error)
	Cat(hash string) (io.ReadCloser, error)
	Get(hash, path string) error
	Pin(hash string, provCtx *IPFSProviderContext) error
	Unpin(hash string, provCtx *IPFSProviderContext) error
}

// DirectIPFSOperations wraps the IPFS shell for direct operations
type DirectIPFSOperations struct {
	ipfs *ipfsnode.Shell
}

// NewDirectIPFSOperations creates a new direct IPFS operations wrapper
func NewDirectIPFSOperations(ipfs *ipfsnode.Shell) *DirectIPFSOperations {
	return &DirectIPFSOperations{ipfs: ipfs}
}

func (d *DirectIPFSOperations) Add(data io.Reader, _ *IPFSProviderContext, opts ...ipfsnode.AddOpts) (string, error) {
	return d.ipfs.Add(data, opts...)
}

func (d *DirectIPFSOperations) Cat(hash string) (io.ReadCloser, error) {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = d.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return nil, err
		}

		return d.ipfs.Cat(hash)
	}

	return d.ipfs.Cat(hash)
}

func (d *DirectIPFSOperations) Get(hash, path string) error {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = d.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}

		return d.ipfs.Get(hash, path)
	}

	return d.ipfs.Get(hash, path)
}

func (d *DirectIPFSOperations) Pin(hash string, _ *IPFSProviderContext) error {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = d.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}

		return d.ipfs.Pin(hash)
	}

	return d.ipfs.Pin(hash)
}

func (d *DirectIPFSOperations) Unpin(hash string, _ *IPFSProviderContext) error {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = d.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}

		return d.ipfs.Unpin(hash)
	}

	return d.ipfs.Unpin(hash)
}
