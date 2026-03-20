// Package block provides stub implementations of the deprecated block package.
// All functions return zero/nil values to allow compilation while block-based
// logic is migrated to PostgreSQL (Phase 07).
package block

import "github.com/rubixchain/rubixgoplatform/types"

const (
	TokenSelfTransferredType  = "TokenSelfTransferred"
	TokenPledgedType          = "TokenPledged"
	TokenUnpledgedType        = "TokenUnpledged"
	TokenGeneratedType        = "TokenGenerated"
	TokenTransferredType      = "TokenTransferred"
	TokenBurntType            = "TokenBurnt"
	TokenDeployedType         = "TokenDeployed"
	TokenExecutedType         = "TokenExecuted"
	TokenMigratedType         = "TokenMigrated"
	TokenMintedType           = "TokenMinted"
	TokenIsBurntForFT         = "TokenIsBurntForFT"
	TokenCommittedType        = "TokenCommitted"
	TokenContractCommited     = "TokenContractCommited"
	TokenPinnedAsService      = "TokenPinnedAsService"
)

// TransTokens holds a token and its type for a block transaction.
type TransTokens struct {
	Token     string
	TokenType int
}

// TransInfo holds metadata for a block transaction.
type TransInfo struct {
	Comment string
	RefID   string
	Tokens  []TransTokens
}

// TokenChainBlock is the input structure used to create a new block.
type TokenChainBlock struct {
	BlockType  string
	TokenOwner string
	TransInfo  *TransInfo
	TokenValue float64
	Epoch      int64
	Version    int
}

// Block is a stub for the deprecated block.Block type.
// TODO(phase07): replace with DB tokenchain queries.
type Block struct{}

type noSignatureOpt struct{}

// NoSignature returns a no-op option for InitBlock.
func NoSignature() interface{} { return noSignatureOpt{} }

// InitBlock returns nil — block parsing is removed.
// TODO(phase07): callers should query DB tokenchain instead.
func InitBlock(data []byte, opts interface{}, extra ...interface{}) *Block {
	return nil
}

// CreateNewBlock returns nil — block creation is removed.
// TODO(phase07): callers should write to DB tokenchain instead.
func CreateNewBlock(ctcb map[string]*Block, tcb *TokenChainBlock) *Block {
	return nil
}

// GetBlockID returns the block ID for a given token.
func (b *Block) GetBlockID(token string) (string, error) { return "", nil }

// GetBlockNumber returns the block height for a given token.
func (b *Block) GetBlockNumber(token string) (uint64, error) { return 0, nil }

// GetPrevBlockID returns the previous block ID for a given token.
func (b *Block) GetPrevBlockID(token string) (string, error) { return "", nil }

// GetTransTokens returns the tokens involved in this block's transaction.
func (b *Block) GetTransTokens() []TransTokens { return nil }

// GetSignature returns the block signature using the provided DID crypto.
func (b *Block) GetSignature(dc types.DIDCrypto) ([]byte, error) { return nil, nil }

// UpdateSignature updates the block signature using the provided DID crypto.
func (b *Block) UpdateSignature(dc types.DIDCrypto) error { return nil }

// GetHash returns the hash of the block.
func (b *Block) GetHash() (string, error) { return "", nil }

// GetOwner returns the DID of the token owner recorded in this block.
func (b *Block) GetOwner() string { return "" }

// GetSenderDID returns the sender DID recorded in this block.
func (b *Block) GetSenderDID() string { return "" }

// GetComment returns the comment recorded in this block.
func (b *Block) GetComment() string { return "" }

// GetBlock returns the raw block bytes.
func (b *Block) GetBlock() []byte { return nil }

// GetTid returns the transaction ID recorded in this block.
func (b *Block) GetTid() string { return "" }

// GetTransType returns the transaction type string recorded in this block.
func (b *Block) GetTransType() string { return "" }

// GetTokenValue returns the token value recorded in this block.
func (b *Block) GetTokenValue() float64 { return 0 }

// GetDeployerDID returns the deployer DID recorded in this block.
func (b *Block) GetDeployerDID() string { return "" }

// GetSigner returns the list of signer DIDs for this block.
func (b *Block) GetSigner() ([]string, error) { return nil, nil }

// GetGenesisNetworkType returns the network type recorded in the genesis block.
func (b *Block) GetGenesisNetworkType(token string) (string, error) { return "", nil }

// GetParentDetials returns the parent token ID for a partial/child token.
// Note: "Detials" typo preserved for compatibility.
func (b *Block) GetParentDetials(token string) (string, error) { return "", nil }
