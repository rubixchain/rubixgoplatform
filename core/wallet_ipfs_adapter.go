package core

import (
	"bytes"
	"io"
	"strings"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/types"
)

// WalletIPFSAdapter adapts Core's IPFSOperations to wallet's types.IPFSOperations interface
type WalletIPFSAdapter struct {
	ops *IPFSOperations
}

// NewWalletIPFSAdapter creates a new adapter
func NewWalletIPFSAdapter(ops *IPFSOperations) types.IPFSOperations {
	return &WalletIPFSAdapter{ops: ops}
}

func (w *WalletIPFSAdapter) Add(data io.Reader, provCtx *types.IPFSProviderContext, opts ...ipfsnode.AddOpts) (string, error) {
	return w.ops.Add(data, provCtx, opts...)
}

func (w *WalletIPFSAdapter) Cat(hash string) (io.ReadCloser, error) {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = w.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return nil, err
		}

		return w.ops.Cat(hash)
	}

	return w.ops.Cat(hash)
}

func (w *WalletIPFSAdapter) Get(hash, path string) error {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = w.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}

		return w.ops.Get(hash, path)
	}

	return w.ops.Get(hash, path)
}

func (w *WalletIPFSAdapter) Pin(hash string, provCtx *types.IPFSProviderContext) error {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = w.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}

		return w.ops.Pin(hash, provCtx)
	}

	return w.ops.Pin(hash, provCtx)
}

func (w *WalletIPFSAdapter) Unpin(hash string, provCtx *types.IPFSProviderContext) error {
	if !(strings.HasPrefix(hash, "Qm") || strings.HasPrefix(hash, "bafy")) {
		var err error
		hash, err = w.Add(bytes.NewBufferString(hash), nil, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
		if err != nil {
			return err
		}

		return w.ops.Unpin(hash, provCtx)
	}

	return w.ops.Unpin(hash, provCtx)
}
