package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// IPFSProviderRecord represents a row in the ipfs_providers table.
type IPFSProviderRecord struct {
	ID            int64
	CID           string
	PeerID        string
	DID           string
	Role          int
	Operation     string
	Status        string
	TransactionID string
	ResourceType  string
	ResourceID    string
	Initiator     string
	Owner         string
	TokenValue    float64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IPFSProviderStore handles persistence of IPFS provider tracking records.
type IPFSProviderStore struct {
	pool   *pgxpool.Pool
	log    logger.Logger
	getPID func() string // returns the node's peer ID (may be empty before RunIPFS)
}

// NewIPFSProviderStore creates a provider store.
// getPeerID is a closure that returns the current peer ID from Core.
func NewIPFSProviderStore(pool *pgxpool.Pool, log logger.Logger, getPeerID func() string) *IPFSProviderStore {
	return &IPFSProviderStore{
		pool:   pool,
		log:    log.Named("ipfs-provider-store"),
		getPID: getPeerID,
	}
}

// RecordProvider inserts a provider record into ipfs_providers.
// If provCtx is nil, this is a no-op. Errors are logged but not returned
// to avoid failing the IPFS operation itself.
func (s *IPFSProviderStore) RecordProvider(cid string, provCtx *types.IPFSProviderContext, operation string) {
	if provCtx == nil {
		return
	}

	peerID := s.getPID()
	if peerID == "" {
		s.log.Error("peer ID not yet available, skipping provider record", "cid", cid)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO ipfs_providers (cid, peer_id, did, role, operation, status, transaction_id, resource_type, resource_id, initiator, owner, token_value, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())`,
		cid, peerID, provCtx.DID, provCtx.Role, operation, constants.IPFSProviderStatusActive,
		provCtx.TransactionID, provCtx.ResourceType, provCtx.ResourceID,
		provCtx.Initiator, provCtx.Owner, provCtx.TokenValue,
	)
	if err != nil {
		s.log.Error("failed to record IPFS provider", "cid", cid, "operation", operation, "error", err)
	}
}

// MarkUnpinned updates provider records for a CID to 'unpinned' status.
// Errors are logged but not returned.
func (s *IPFSProviderStore) MarkUnpinned(cid string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`UPDATE ipfs_providers SET status = $1, updated_at = NOW() WHERE cid = $2 AND status = $3`,
		constants.IPFSProviderStatusUnpinned, cid, constants.IPFSProviderStatusActive,
	)
	if err != nil {
		s.log.Error("failed to mark provider as unpinned", "cid", cid, "error", err)
	}
}

// GetProviderByCID retrieves the most recent provider record for a CID.
func (s *IPFSProviderStore) GetProviderByCID(cid string) (*IPFSProviderRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var r IPFSProviderRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, cid, peer_id, did, role, operation, status, transaction_id, resource_type, resource_id, initiator, owner, token_value, created_at, updated_at
		 FROM ipfs_providers WHERE cid = $1 ORDER BY created_at DESC LIMIT 1`, cid).
		Scan(&r.ID, &r.CID, &r.PeerID, &r.DID, &r.Role, &r.Operation, &r.Status,
			&r.TransactionID, &r.ResourceType, &r.ResourceID, &r.Initiator, &r.Owner, &r.TokenValue,
			&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		//If no rows are found, return nil
		if err == pgx.ErrNoRows {
			s.log.Debug("no provider record found for cid", "cid", cid)
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

