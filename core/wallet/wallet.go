package wallet

import (
	"context"
	"fmt"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// Querier is the common interface satisfied by both *pgxpool.Pool and pgx.Tx.
// Methods that accept ...pgx.Tx use this to run inside or outside a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// q returns the provided transaction if given, otherwise the connection pool.
func (w *Wallet) q(tx ...pgx.Tx) Querier {
	if len(tx) > 0 && tx[0] != nil {
		return tx[0]
	}
	return w.db.Pool()
}

type Wallet struct {
	ipfs    *ipfsnode.Shell
	ipfsOps types.IPFSOperations
	log     logger.Logger
	db      *storage.RubixDB
	Ctx     context.Context
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
