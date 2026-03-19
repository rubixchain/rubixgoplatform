// Package contract is a temporary compilation stub. The original contract
// package was deleted as part of the PostgreSQL migration. These types satisfy
// the 11 files that still import this package. They will be replaced with
// models.* equivalents during Phase 07 (Contract Removal).
// TODO(phase07): Replace all contract.TokenInfo usages with models equivalents and delete this package.
package contract

// TokenInfo holds per-token details passed through the consensus and quorum paths.
// Fields mirror the original contract.TokenInfo struct used across all 11 consuming files.
type TokenInfo struct {
	Token      string
	TokenType  int
	TokenValue float64
	OwnerDID   string
	BlockID    string
}

// TransInfo carries sender/receiver/token metadata for a contract transaction.
type TransInfo struct {
	SenderDID          string
	ReceiverDID        string
	DeployerDID        string
	ExecutorDID        string
	Comment            string
	TransTokens        []TokenInfo
	CommitedTokens     []TokenInfo
	NFT                string
	NFTData            string
	NFTValue           float64
	SmartContractToken string
	SmartContractData  string
}

// ContractType describes the type and pledge mode of a contract transaction.
type ContractType struct {
	Type       int
	PledgeMode int
	TotalRBTs  float64
	TransInfo  *TransInfo
	ReqID      string
}

// Contract is a stub for the original contract.Contract consensus object.
type Contract struct {
	block        []byte
	contractType *ContractType
}

// Contract type constants.
const (
	SCFTType             = 1
	SCNFTType            = 2
	NFTDeployType        = 3
	NFTExecuteType       = 4
	SmartContractDeployType = 5
)

// Pledge mode constants.
const (
	PeriodicPledgeMode = 1
)

// InitContract constructs a Contract from a serialised block and a peer interface.
// TODO(phase07): implement real contract initialisation from block bytes.
func InitContract(block []byte, p interface{}) *Contract {
	// TODO(phase07): deserialise block into Contract fields.
	return &Contract{block: block}
}

// CreateNewContract constructs a Contract from a ContractType descriptor.
// TODO(phase07): implement real contract creation.
func CreateNewContract(ct *ContractType) *Contract {
	// TODO(phase07): populate Contract fields from ct.
	return &Contract{contractType: ct}
}

// GetSenderDID returns the sender DID from the contract's TransInfo.
func (c *Contract) GetSenderDID() string {
	// TODO(phase07): replace with real field access after migration.
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.SenderDID
	}
	return ""
}

// GetReceiverDID returns the receiver DID from the contract's TransInfo.
func (c *Contract) GetReceiverDID() string {
	// TODO(phase07): replace with real field access after migration.
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.ReceiverDID
	}
	return ""
}

// GetDeployerDID returns the deployer DID from the contract's TransInfo.
func (c *Contract) GetDeployerDID() string {
	// TODO(phase07): replace with real field access after migration.
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.DeployerDID
	}
	return ""
}

// GetExecutorDID returns the executor DID from the contract's TransInfo.
func (c *Contract) GetExecutorDID() string {
	// TODO(phase07): replace with real field access after migration.
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.ExecutorDID
	}
	return ""
}

// GetTransTokenInfo returns the TransTokens slice from the contract's TransInfo.
func (c *Contract) GetTransTokenInfo() []TokenInfo {
	// TODO(phase07): replace with real field access after migration.
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.TransTokens
	}
	return nil
}

// GetCommitedTokensInfo returns the CommitedTokens slice from the contract's TransInfo.
func (c *Contract) GetCommitedTokensInfo() []TokenInfo {
	// TODO(phase07): replace with real field access after migration.
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.CommitedTokens
	}
	return nil
}

// GetBlock returns the raw serialised block bytes held by this contract.
func (c *Contract) GetBlock() []byte {
	// TODO(phase07): replace with real block serialisation.
	return c.block
}

// VerifySignature is a stub that always returns nil.
// TODO(phase07): implement real signature verification using util.VerifySignature.
func (c *Contract) VerifySignature(dc interface{}) error {
	// TODO(phase07): implement real signature verification
	return nil
}

// UpdateSignature is a stub that always returns nil.
// TODO(phase07): implement real signature update using util.SignTransaction.
func (c *Contract) UpdateSignature(dc interface{}) error {
	// TODO(phase07): implement real signature update
	return nil
}
