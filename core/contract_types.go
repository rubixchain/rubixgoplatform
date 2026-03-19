// Package core — contract_types.go
// Replacement types for the former contract package, internalized into the core package.
// These replace contract.TokenInfo, contract.TransInfo, contract.ContractType, and contract.Contract
// so the external contract/ directory can be deleted.
// TODO(phase07): Implement real contract logic when the block package migration completes.
package core

// ContractTokenInfo holds per-token details passed through consensus and quorum paths.
// Replaces the former contract.TokenInfo struct.
type ContractTokenInfo struct {
	Token      string
	TokenType  int
	TokenValue float64
	OwnerDID   string
	BlockID    string
}

// ContractTransInfo carries sender/receiver/token metadata for a contract transaction.
// Replaces the former contract.TransInfo struct.
type ContractTransInfo struct {
	SenderDID          string
	ReceiverDID        string
	DeployerDID        string
	ExecutorDID        string
	Comment            string
	TransTokens        []ContractTokenInfo
	CommitedTokens     []ContractTokenInfo
	NFT                string
	NFTData            string
	NFTValue           float64
	SmartContractToken string
	SmartContractData  string
}

// ContractTypeInfo describes the type and pledge mode of a contract transaction.
// Replaces the former contract.ContractType struct.
type ContractTypeInfo struct {
	Type       int
	PledgeMode int
	TotalRBTs  float64
	TransInfo  *ContractTransInfo
	ReqID      string
}

// ConsensusContract is the stub for contract verification during consensus.
// Replaces the former contract.Contract struct.
type ConsensusContract struct {
	block        []byte
	contractType *ContractTypeInfo
}

// Contract type constants (moved from the former contract package).
const (
	SCFTType                = 1
	SCNFTType               = 2
	NFTDeployType           = 3
	NFTExecuteType          = 4
	SmartContractDeployType = 5
)

// Pledge mode constants (moved from the former contract package).
const (
	PeriodicPledgeMode = 1
)

// InitConsensusContract constructs a ConsensusContract from serialised block bytes and a peer interface.
// TODO(phase07): implement real contract initialisation from block bytes.
func InitConsensusContract(block []byte, p interface{}) *ConsensusContract {
	// TODO(phase07): deserialise block into ConsensusContract fields.
	return &ConsensusContract{block: block}
}

// CreateNewConsensusContract constructs a ConsensusContract from a ContractTypeInfo descriptor.
// TODO(phase07): implement real contract creation.
func CreateNewConsensusContract(ct *ContractTypeInfo) *ConsensusContract {
	// TODO(phase07): populate ConsensusContract fields from ct.
	return &ConsensusContract{contractType: ct}
}

// GetSenderDID returns the sender DID from the contract's TransInfo.
// TODO(phase07): replace with real field access after migration.
func (c *ConsensusContract) GetSenderDID() string {
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.SenderDID
	}
	return ""
}

// GetReceiverDID returns the receiver DID from the contract's TransInfo.
// TODO(phase07): replace with real field access after migration.
func (c *ConsensusContract) GetReceiverDID() string {
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.ReceiverDID
	}
	return ""
}

// GetDeployerDID returns the deployer DID from the contract's TransInfo.
// TODO(phase07): replace with real field access after migration.
func (c *ConsensusContract) GetDeployerDID() string {
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.DeployerDID
	}
	return ""
}

// GetExecutorDID returns the executor DID from the contract's TransInfo.
// TODO(phase07): replace with real field access after migration.
func (c *ConsensusContract) GetExecutorDID() string {
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.ExecutorDID
	}
	return ""
}

// GetTransTokenInfo returns the TransTokens slice from the contract's TransInfo.
// TODO(phase07): replace with real field access after migration.
func (c *ConsensusContract) GetTransTokenInfo() []ContractTokenInfo {
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.TransTokens
	}
	return nil
}

// GetCommitedTokensInfo returns the CommitedTokens slice from the contract's TransInfo.
// TODO(phase07): replace with real field access after migration.
func (c *ConsensusContract) GetCommitedTokensInfo() []ContractTokenInfo {
	if c.contractType != nil && c.contractType.TransInfo != nil {
		return c.contractType.TransInfo.CommitedTokens
	}
	return nil
}

// GetBlock returns the raw serialised block bytes held by this contract.
// TODO(phase07): replace with real block serialisation.
func (c *ConsensusContract) GetBlock() []byte {
	return c.block
}

// VerifySignature is a stub that always returns nil.
// TODO(phase07): implement real signature verification using util.VerifySignature.
func (c *ConsensusContract) VerifySignature(dc interface{}) error {
	// TODO(phase07): implement real signature verification
	return nil
}

// UpdateSignature is a stub that always returns nil.
// TODO(phase07): implement real signature update using util.SignTransaction.
func (c *ConsensusContract) UpdateSignature(dc interface{}) error {
	// TODO(phase07): implement real signature update
	return nil
}
