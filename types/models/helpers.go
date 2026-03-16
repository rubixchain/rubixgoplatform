package models

// GetRBTAmount returns the RBT amount to be transferred
func (tr *TransactionRequest) GetRBTAmount() float64 {
	return tr.Tokens.RBT
}

// HasRBT checks if the transfer includes RBT tokens
func (tr *TransactionRequest) HasRBT() bool {
	return tr.Tokens.RBT > 0
}

// GetAllFTs returns all FT info in the transfer
func (tr *TransactionRequest) GetAllFTs() []FTInfo {
	return tr.Tokens.FT
}

// HasFT checks if the transfer includes fungible tokens
func (tr *TransactionRequest) HasFT() bool {
	return len(tr.Tokens.FT) > 0
}

// GetAllNFTs returns all NFT info in the transfer
func (tr *TransactionRequest) GetAllNFTs() []NFTInfo {
	return tr.Tokens.NFT
}

// HasNFT checks if the transfer includes NFTs
func (tr *TransactionRequest) HasNFT() bool {
	return len(tr.Tokens.NFT) > 0
}

// GetAllSmartContracts returns all smart contract info in the transfer
func (tr *TransactionRequest) GetAllSmartContracts() []SmartContractInfo {
	return tr.Tokens.SmartContract
}

// HasSmartContract checks if the transfer includes smart contracts
func (tr *TransactionRequest) HasSmartContract() bool {
	return len(tr.Tokens.SmartContract) > 0
}
