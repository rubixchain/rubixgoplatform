package wallet

// TODO(phase07): all types and methods in this file are stubs pending DB implementation.

// UnpledgeSequenceInfo is a Go-native representation of an unpledge sequence record.
type UnpledgeSequenceInfo struct {
	TransactionID string
	Epoch         int64
	PledgeTokens  string
	QuorumDID     string
}

// PledgeInformation holds pledge/unpledge identifiers for a single pledged token.
type PledgeInformation struct {
	TokenID         string
	TokenType       int
	PledgeBlockID   string
	UnpledgeBlockID string
	QuorumDID       string
	TransactionID   string
}

// GetUnpledgeSequenceInfos returns all pending unpledge sequences from the DB.
func (w *Wallet) GetUnpledgeSequenceInfos() ([]UnpledgeSequenceInfo, error) {
	// TODO(phase07): query unpledge_sequence table
	return nil, nil
}

// GetUnpledgeSequenceDetails is an alias for GetUnpledgeSequenceInfos.
func (w *Wallet) GetUnpledgeSequenceDetails() ([]UnpledgeSequenceInfo, error) {
	return w.GetUnpledgeSequenceInfos()
}

// GetTokenStateHashByTransactionID returns token state hash records for a transaction.
func (w *Wallet) GetTokenStateHashByTransactionID(transactionID string) ([]string, error) {
	// TODO(phase07): query token_state_hash table by transaction_id
	return nil, nil
}

// StoreCredit persists credit records earned from unpledging.
func (w *Wallet) StoreCredit(transactionID string, quorumDID string, pledgeInfo []*PledgeInformation) error {
	// TODO(phase07): insert credit records into DB
	return nil
}

// RemoveUnpledgeSequenceInfo deletes the unpledge sequence record for a transaction.
func (w *Wallet) RemoveUnpledgeSequenceInfo(transactionID string) error {
	// TODO(phase07): delete from unpledge_sequence where transaction_id=$1
	return nil
}

// RemoveCredit removes stored credit for a transaction (rollback path).
func (w *Wallet) RemoveCredit(transactionID string) error {
	// TODO(phase07): delete from credits where transaction_id=$1
	return nil
}

// UpdateUnpledgedTokenStatus marks a token as unpledged in the DB.
func (w *Wallet) UpdateUnpledgedTokenStatus(quorumDID string, token string, tokenType int) error {
	// TODO(phase07): update token status in tokens table
	return nil
}

// CreateTokenBlock is a no-op stub; token chain blocks now persisted via DB.
func (w *Wallet) CreateTokenBlock(blk *BlockStub) error {
	// TODO(phase07): block-based; no-op stub
	return nil
}

// UnpledgeWholeToken updates a token's pledge state to unpledged in the DB.
func (w *Wallet) UnpledgeWholeToken(quorumDID string, token string, tokenType int) error {
	// TODO(phase07): update token pledge status in DB
	return nil
}

// RemoveTokenStateHashByTransactionID removes all token state hash records for a transaction.
func (w *Wallet) RemoveTokenStateHashByTransactionID(transactionID string) error {
	// TODO(phase07): delete from token_state_hash where transaction_id=$1
	return nil
}
