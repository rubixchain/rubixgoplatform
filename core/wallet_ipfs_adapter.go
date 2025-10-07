package core

import (
	"io"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// WalletIPFSAdapter adapts Core's IPFSOperations to wallet's IPFSOperations interface
type WalletIPFSAdapter struct {
	ops *IPFSOperations
}

// NewWalletIPFSAdapter creates a new adapter
func NewWalletIPFSAdapter(ops *IPFSOperations) wallet.IPFSOperations {
	return &WalletIPFSAdapter{ops: ops}
}

func (w *WalletIPFSAdapter) Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error) {
	return w.ops.Add(data, opts...)
}

func (w *WalletIPFSAdapter) Cat(hash string) (io.ReadCloser, error) {
	return w.ops.Cat(hash)
}

func (w *WalletIPFSAdapter) Get(hash, path string) error {
	return w.ops.Get(hash, path)
}

func (w *WalletIPFSAdapter) Pin(hash string) error {
	return w.ops.Pin(hash)
}

func (w *WalletIPFSAdapter) Unpin(hash string) error {
	return w.ops.Unpin(hash)
}