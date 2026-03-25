package wallet

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// TODO(phase07): block-based wallet methods removed; use DB tokenchain

// BlockStub is a placeholder type replacing block.Block from the removed block package.
// All Get* wallet methods return nil (*BlockStub), so no method is ever called.
type BlockStub struct{}

// GetLatestTokenBlock was block-based; callers must use DB queries.
func (w *Wallet) GetLatestTokenBlock(token string, tokenType int) *BlockStub {
	return nil
}

// GetGenesisTokenBlock was block-based; callers must use DB queries.
func (w *Wallet) GetGenesisTokenBlock(token string, tokenType int) *BlockStub {
	return nil
}

// GetFullNodeLatestTokenBlock was block-based; callers must use DB queries.
func (w *Wallet) GetFullNodeLatestTokenBlock(token string, tokenType int) *BlockStub {
	return nil
}

// GetFullNodeGenesisTokenBlock was block-based; callers must use DB queries.
func (w *Wallet) GetFullNodeGenesisTokenBlock(token string, tokenType int) *BlockStub {
	return nil
}

// AddTokenBlock was block-based; token blocks now persisted via DB.
func (w *Wallet) AddTokenBlock(token string, blk *BlockStub) error {
	// TODO(phase07): block storage removed; use DB tokenchain
	return nil
}

// AddFullNodeTokenBlock was block-based; token blocks now persisted via DB.
func (w *Wallet) AddFullNodeTokenBlock(token string, blk *BlockStub) error {
	// TODO(phase07): block storage removed; use DB tokenchain
	return nil
}

// GetTokenBlock returns the raw block bytes for a given token, type, and block ID.
func (w *Wallet) GetTokenBlock(token string, tokenType int, blockID string) ([]byte, error) {
	// TODO(phase07): query tokenchain table for block bytes
	return nil, nil
}

// AddSyncedRBTToTable stubs adding an RBT sync record (fullnode).
func (w *Wallet) AddSyncedRBTToTable(info *SyncedRBT) error {
	// TODO(phase07): implement using PostgreSQL
	return nil
}

// UpdateSyncedRBTToTable stubs updating an RBT sync record (fullnode).
func (w *Wallet) UpdateSyncedRBTToTable(info *SyncedRBT) error {
	// TODO(phase07): implement using PostgreSQL
	return nil
}

// AddDIDPeerMap adds a DID->PeerID mapping to the peer DID table.
func (w *Wallet) AddDIDPeerMap(did string, peerID string) {
	// TODO(phase07): insert into peer_did table
}

// GetNFTTokensByOwner returns NFT tokens owned by a given DID.
func (w *Wallet) GetNFTTokensByOwner(did string) ([]*models.TokenInfo, error) {
	// TODO(phase07): query tokens table by did and token_type=nft
	return nil, nil
}
