package wallet

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// ── Token content types ───────────────────────────────────────────────────────

// RBTContent holds the IPFS content bytes for an RBT token.
type RBTContent struct {
	TokenID    string
	RBTContent string
}

// FTContent holds the IPFS content bytes for an FT token.
type FTContent struct {
	TokenID   string
	FTContent string
}

// TokenStateHashRecord holds a pledged token state record.
type TokenStateHashRecord struct {
	DID            string
	PledgedTokens  string
	TokenStateHash string
}

// ── Token number / level stubs ────────────────────────────────────────────────

// GetLocalTokenNumber returns the current local RBT token number counter.
// TODO(phase07): implement using PostgreSQL token_meta table.
func (w *Wallet) GetLocalTokenNumber() (int, error) {
	return 0, nil
}

// SetLocalTokenlevelAndNumber persists (tokenLevel, numInLevel) after minting.
// TODO(phase07): implement using PostgreSQL token_meta table.
func (w *Wallet) SetLocalTokenlevelAndNumber(tokenLevel int, numInLevel int) error {
	return nil
}

// ── DID list stub ─────────────────────────────────────────────────────────────

// GetAllDIDs returns all DIDs stored in the wallet.
// TODO(phase07): implement using PostgreSQL did table.
func (w *Wallet) GetAllDIDs() ([]models.DID, error) {
	return nil, nil
}

// ── Token status stubs ────────────────────────────────────────────────────────

// UpdateTokenStatus updates the status of a single token.
// TODO(phase07): implement using PostgreSQL tokens table.
func (w *Wallet) UpdateTokenStatus(did string, tokenHash string, tokenType int, newStatus int) error {
	return nil
}

// ── Token state hash stubs ────────────────────────────────────────────────────

// GetAllTokenStateHash retrieves all pledged token state hash records.
// TODO(phase07): implement using PostgreSQL token_state_hashes table.
func (w *Wallet) GetAllTokenStateHash() ([]TokenStateHashRecord, error) {
	return nil, nil
}

// ── Tokens to sync stubs ──────────────────────────────────────────────────────

// GetTokensToBeSynced returns tokens whose sync is incomplete.
// TODO(phase07): implement using PostgreSQL tokens table.
func (w *Wallet) GetTokensToBeSynced() ([]models.Token, error) {
	return nil, nil
}

// ── Transaction history stub ──────────────────────────────────────────────────

// GetTransactionDetailsbyTransactionId returns transaction details by ID.
// TODO(phase07): implement using PostgreSQL transaction_history table.
func (w *Wallet) GetTransactionDetailsbyTransactionId(transactionID string) (*model.TransactionDetails, error) {
	return nil, nil
}

// ── IPFS content fetch stub ───────────────────────────────────────────────────

// Cat fetches IPFS content by hash using the given role and peerID.
// TODO(phase07): implement using IPFS and provider map.
func (w *Wallet) Cat(hash string, role int, peerID string) (string, error) {
	return "", nil
}

// ── RBT content stubs ────────────────────────────────────────────────────────

// AddRBTContentToPSQl persists RBT IPFS content to PostgreSQL.
// TODO(phase09): implement storage.
func (w *Wallet) AddRBTContentToPSQl(content *RBTContent) error {
	return nil
}

// ReadRBTContentFromTable reads RBT content from PostgreSQL by token ID.
// TODO(phase09): implement storage.
func (w *Wallet) ReadRBTContentFromTable(tokenID string) (*RBTContent, error) {
	return nil, nil
}

// ── FT content stubs ──────────────────────────────────────────────────────────

// AddFTContentToPSQl persists FT IPFS content to PostgreSQL.
// TODO(phase09): implement storage.
func (w *Wallet) AddFTContentToPSQl(content *FTContent) error {
	return nil
}

// ReadFTContentFromTable reads FT content from PostgreSQL by token ID.
// TODO(phase09): implement storage.
func (w *Wallet) ReadFTContentFromTable(tokenID string) (*FTContent, error) {
	return nil, nil
}

// ── NFT content stubs ─────────────────────────────────────────────────────────

// StoreNFTFilesToPSQL persists NFT files to PostgreSQL.
// TODO(phase09): implement storage.
func (w *Wallet) StoreNFTFilesToPSQL(tokenID string, did string, artifactHash string, outputDir string) error {
	return nil
}

// ReadNFTContentFromTable reads NFT content from PostgreSQL by token ID.
// TODO(phase09): implement storage.
func (w *Wallet) ReadNFTContentFromTable(tokenID string) (interface{}, error) {
	return nil, nil
}

// ── Smart contract content stubs ──────────────────────────────────────────────

// ReadSmartContractContentFromTable reads smart contract content from PostgreSQL.
// TODO(phase09): implement storage.
func (w *Wallet) ReadSmartContractContentFromTable(tokenID string) (interface{}, error) {
	return nil, nil
}

// ── Token lock/release stubs ──────────────────────────────────────────────────

// ReleaseToken releases a single locked token.
// TODO(phase07): implement using PostgreSQL tokens table.
func (w *Wallet) ReleaseToken(token string) {
}
