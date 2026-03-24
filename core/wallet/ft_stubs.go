package wallet

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// ── Legacy storage constants ──────────────────────────────────────────────────

const (
	FTTokenStorage = "ft_token_storage"
	FTStorage      = "ft_storage"

	// OwnerRole is the IPFS pin role for a token owner (int, matches token_provider_map.go role param).
	OwnerRole = 0
)

// ── Legacy stub types ─────────────────────────────────────────────────────────

// FTToken is a legacy stub representing a fungible token record from the old SQLite schema.
type FTToken struct {
	TokenID       string
	FTName        string
	TokenStatus   int
	TokenValue    float64
	DID           string
	CreatorDID    string
	TransactionID string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// FT is a legacy stub representing an FT aggregate record.
type FT struct {
	FTName     string
	CreatorDID string
	FTCount    int
}

// FTTokenFixResult is a legacy stub for FT fix operation results.
type FTTokenFixResult struct {
	TokenID    string
	OldDID     string
	NewDID     string
	FixApplied bool
	// Fields expected by server/ft.go
	OldCreator string
	NewCreator string
	Success    bool
	Error      error
}

// LegacyStorageStub replaces the old SQLite storage interface with no-ops.
type LegacyStorageStub struct{}

func (s *LegacyStorageStub) Read(storage string, out interface{}, conditions ...interface{}) error {
	return fmt.Errorf("LegacyStorageStub.Read: not implemented (storage: %s)", storage)
}

func (s *LegacyStorageStub) Write(storage string, in interface{}) error {
	return fmt.Errorf("LegacyStorageStub.Write: not implemented (storage: %s)", storage)
}

func (s *LegacyStorageStub) Update(storage string, in interface{}, conditions ...interface{}) error {
	return fmt.Errorf("LegacyStorageStub.Update: not implemented (storage: %s)", storage)
}

func (s *LegacyStorageStub) Delete(storage string, in interface{}, conditions ...interface{}) error {
	return fmt.Errorf("LegacyStorageStub.Delete: not implemented (storage: %s)", storage)
}

func (s *LegacyStorageStub) WriteBatch(storage string, in interface{}, batchSize int) error {
	return fmt.Errorf("LegacyStorageStub.WriteBatch: not implemented (storage: %s)", storage)
}

func (w *Wallet) S() *LegacyStorageStub {
	return &LegacyStorageStub{}
}

func (w *Wallet) GetStorage() *LegacyStorageStub {
	return &LegacyStorageStub{}
}

// ── Synced fullnode types ─────────────────────────────────────────────────────

// SyncedRBT holds fullnode sync state for an RBT token.
type SyncedRBT struct {
	TokenID       string
	OwnerDID      string
	PublisherDID  string
	BlockHash     string
	BlockHeight   uint64
	TransactionID string
	SyncStatus    int
	TokenStatus   int
	TokenValue    float64
}

// SyncedFT holds fullnode sync state for an FT token.
type SyncedFT struct {
	TokenID       string
	OwnerDID      string
	CreatorDID    string
	PublisherDID  string
	BlockHash     string
	BlockHeight   uint64
	TransactionID string
	SyncStatus    int
	TokenStatus   int
	TokenValue    float64
	FTName        string
}

// SyncedNFT holds fullnode sync state for an NFT token.
type SyncedNFT struct {
	TokenID       string
	OwnerDID      string
	PublisherDID  string
	BlockHash     string
	BlockHeight   uint64
	TransactionID string
	SyncStatus    int
	TokenStatus   int
	TokenValue    float64
}

// SyncedSmartContract holds fullnode sync state for a smart contract token.
type SyncedSmartContract struct {
	SmartContractHash string
	Deployer          string
	PublisherDID      string
	BlockHash         string
	BlockHeight       uint64
	TransactionID     string
	SyncStatus        int
	TokenStatus       int
}

// ── Stub wallet methods ───────────────────────────────────────────────────────

func (w *Wallet) GetFreeFTsByNameAndCreatorDID(ftName, ownerDID, creatorDID string) ([]FTToken, error) {
	return nil, fmt.Errorf("GetFreeFTsByNameAndCreatorDID: not implemented")
}

func (w *Wallet) GetFreeFTsByNameAndDID(ftName, ownerDID string) ([]FTToken, error) {
	return nil, fmt.Errorf("GetFreeFTsByNameAndDID: not implemented")
}

func (w *Wallet) GetFTsAndCount(did string) ([]FT, error) {
	return nil, fmt.Errorf("GetFTsAndCount: not implemented")
}

func (w *Wallet) GetAllFTsAndCount() ([]FT, error) {
	return nil, fmt.Errorf("GetAllFTsAndCount: not implemented")
}

func (w *Wallet) GetLockedFTs() ([]FTToken, error) {
	return nil, fmt.Errorf("GetLockedFTs: not implemented")
}

func (w *Wallet) BatchAddTokenBlocksFT(blockPairs interface{}) error {
	return fmt.Errorf("BatchAddTokenBlocksFT: not implemented")
}

func (w *Wallet) AddProviderDetailsBatch(maps []models.TokenProviderMap) error {
	return fmt.Errorf("AddProviderDetailsBatch: not implemented")
}

func (w *Wallet) GetFTTokenCreatorStats() (map[string]interface{}, error) {
	return nil, fmt.Errorf("GetFTTokenCreatorStats: not implemented")
}

func (w *Wallet) FixAllFTTokensWithPeerIDAsCreator() ([]FTTokenFixResult, error) {
	return nil, fmt.Errorf("FixAllFTTokensWithPeerIDAsCreator: not implemented")
}

func (w *Wallet) AddTransactionHistory(td *model.TransactionDetails) error {
	// TODO(phase09): implement transaction history persistence
	return nil
}

func (w *Wallet) AddFTTransactionHistory(td *model.TransactionDetails, ftName string, creatorDID string, ftCount int) error {
	// TODO(phase09): implement FT transaction history
	return nil
}

func (w *Wallet) AddFTTransactionTokens(txID, creatorDID, ftName string, ftCount int, mode string) error {
	// TODO(phase09): implement FT transaction tokens
	return nil
}

func (w *Wallet) UpdateTokenSyncStatus(tokenID string, status int) error {
	// TODO(phase09): sync_status column not yet in schema
	return nil
}

func (w *Wallet) ReadSyncedRBTFromTable(tokenID string) (*SyncedRBT, error) {
	return nil, fmt.Errorf("ReadSyncedRBTFromTable: not implemented")
}

func (w *Wallet) RemoveSyncedRBTFromTable(tokenID string) error {
	return fmt.Errorf("RemoveSyncedRBTFromTable: not implemented")
}

func (w *Wallet) ReadSyncedFTFromTable(tokenID string) (*SyncedFT, error) {
	return nil, fmt.Errorf("ReadSyncedFTFromTable: not implemented")
}

func (w *Wallet) AddSyncedFTToTable(ftInfo *SyncedFT) error {
	return fmt.Errorf("AddSyncedFTToTable: not implemented")
}

func (w *Wallet) UpdateSyncedFTToTable(syncedFT *SyncedFT) error {
	return fmt.Errorf("UpdateSyncedFTToTable: not implemented")
}

func (w *Wallet) RemoveSyncedFTFromTable(tokenID string) error {
	return fmt.Errorf("RemoveSyncedFTFromTable: not implemented")
}

func (w *Wallet) ReadSyncedNFTFromTable(tokenID string) (*SyncedNFT, error) {
	return nil, fmt.Errorf("ReadSyncedNFTFromTable: not implemented")
}

func (w *Wallet) AddSyncedNFTToTable(nftInfo *SyncedNFT) error {
	return fmt.Errorf("AddSyncedNFTToTable: not implemented")
}

func (w *Wallet) UpdateSyncedNFTToTable(syncedNFT *SyncedNFT) error {
	return fmt.Errorf("UpdateSyncedNFTToTable: not implemented")
}

func (w *Wallet) RemoveSyncedNFTFromTable(tokenID string) error {
	return fmt.Errorf("RemoveSyncedNFTFromTable: not implemented")
}

func (w *Wallet) ReadSyncedSmartContractFromTable(tokenID string) (*SyncedSmartContract, error) {
	return nil, fmt.Errorf("ReadSyncedSmartContractFromTable: not implemented")
}

func (w *Wallet) AddSyncedSmartContractToTable(scInfo *SyncedSmartContract) error {
	return fmt.Errorf("AddSyncedSmartContractToTable: not implemented")
}

func (w *Wallet) UpdateSyncedSmartContractToTable(syncedSC *SyncedSmartContract) error {
	return fmt.Errorf("UpdateSyncedSmartContractToTable: not implemented")
}

func (w *Wallet) RemoveSyncedSmartContractFromTable(tokenID string) error {
	return fmt.Errorf("RemoveSyncedSmartContractFromTable: not implemented")
}

func (w *Wallet) AddDoubleSpentTokenInfo(info *model.DoubleSpentTokenInfo) error {
	return fmt.Errorf("AddDoubleSpentTokenInfo: not implemented")
}

func (w *Wallet) AddFailedTokensToTable(info interface{}) error {
	return fmt.Errorf("AddFailedTokensToTable: not implemented")
}

func (w *Wallet) DeleteFailedToSyncTokenFromTable(tokenID string) error {
	return fmt.Errorf("DeleteFailedToSyncTokenFromTable: not implemented")
}

// ── Fullnode-specific stubs ────────────────────────────────────────────────────

// StoreFailedTransaction stubs storing a permanently-failed transaction for review.
// TODO: implement using PostgreSQL failed_transactions table.
func (w *Wallet) StoreFailedTransaction(txn *model.FailedTransaction) error {
	return nil
}

// GetAllFailedToSyncTokens stubs retrieval of tokens that failed to sync.
// TODO: implement using PostgreSQL.
func (w *Wallet) GetAllFailedToSyncTokens() ([]model.FailedToSyncTokenDetailsInfo, error) {
	return nil, nil
}

// AddTransactionsToFullNodeTransactionHistoryTable stubs inserting a fullnode txn history record.
// TODO: implement using PostgreSQL.
func (w *Wallet) AddTransactionsToFullNodeTransactionHistoryTable(t *model.FullNodeTxnHistoryInfo) error {
	return nil
}

// ReadFullNodeTransactionHistoryTable stubs reading a fullnode txn history record by ID.
// TODO: implement using PostgreSQL.
func (w *Wallet) ReadFullNodeTransactionHistoryTable(transactionID string) (*model.FullNodeTxnHistoryInfo, error) {
	return nil, fmt.Errorf("ReadFullNodeTransactionHistoryTable: not implemented")
}

// UpdateFullNodeTransactionHistoryTable stubs updating a fullnode txn history record.
// TODO: implement using PostgreSQL.
func (w *Wallet) UpdateFullNodeTransactionHistoryTable(t *model.FullNodeTxnHistoryInfo) error {
	return nil
}
