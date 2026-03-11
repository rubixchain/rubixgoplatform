package wallet

import (
	"context"
	"fmt"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

type Wallet struct {
	ipfs             *ipfsnode.Shell
	ipfsOps          types.IPFSOperations
	log              logger.Logger
	asyncProviderMgr *AsyncProviderDetailsManager
	db               *storage.RubixDB
	Ctx              context.Context
}

// GetIpfsOps returns the IPFS operations interface
func (w *Wallet) GetIpfsOps() types.IPFSOperations {
	return w.ipfsOps
}

func NewWallet(ctx context.Context, db *storage.RubixDB, log logger.Logger) (*Wallet, error) {
	w := &Wallet{
		log: log.Named("wallet"),
		db:  db,
		Ctx: ctx,
	}

	err := w.db.InitSchema(w.Ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialise table schema: %v", err)
	}

	err = w.addProtocolValuesToLookupTables()
	if err != nil {
		return nil, fmt.Errorf("failed to add protocol values to lookup tables: %v", err)
	}

	// Initialize async provider details manager with 2 workers
	w.asyncProviderMgr = NewAsyncProviderDetailsManager(w, 2)

	return w, nil
}

func (w *Wallet) SetupWallet(ipfs *ipfsnode.Shell) {
	w.ipfs = ipfs
	// Default to direct IPFS operations if no health-managed operations are set
	if w.ipfsOps == nil {
		w.ipfsOps = types.NewDirectIPFSOperations(ipfs)
	}
}

// SetIPFSOperations sets the IPFS operations interface (for health-managed operations)
func (w *Wallet) SetIPFSOperations(ops types.IPFSOperations) {
	w.ipfsOps = ops
}
