package wallet

import (
	"context"
	"fmt"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

type Wallet struct {
	ipfs    *ipfsnode.Shell
	ipfsOps types.IPFSOperations
	log     logger.Logger
	db      *storage.RubixDB
	Ctx     context.Context
	didDir  string
}

// SetDidDir sets the DID directory so the wallet can instantiate DIDLite for
// pre-persistence signature verification. Call this after NewWallet.
func (w *Wallet) SetDidDir(dir string) {
	w.didDir = dir
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

// BeginTx starts a new database transaction. The caller is responsible for
// committing or rolling back the returned transaction.
func (w *Wallet) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return w.db.BeginTx(ctx)
}
