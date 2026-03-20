package wallet

// TODO(phase07): block-based wallet methods removed; use DB tokenchain

// BlockStub is a temporary placeholder replacing *block.Block.
// All Get* wallet methods return nil (*BlockStub), so no method is ever called.
// These method signatures exist solely to satisfy the compiler.
type BlockStub struct{}

func (b *BlockStub) GetBlockNumber(token string) (uint64, error) { return 0, nil }
func (b *BlockStub) GetBlockID(token string) (string, error)     { return "", nil }
func (b *BlockStub) GetHash() (string, error)                    { return "", nil }
func (b *BlockStub) GetOwner() string                            { return "" }
func (b *BlockStub) GetTid() string                              { return "" }
func (b *BlockStub) GetTransType() string                        { return "" }
func (b *BlockStub) GetTokenValue() float64                      { return 0 }
func (b *BlockStub) GetBlock() []byte                            { return nil }
func (b *BlockStub) GetComment() string                          { return "" }
func (b *BlockStub) GetDeployerDID() string                      { return "" }
func (b *BlockStub) GetSigner() ([]string, error)                { return nil, nil }

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
