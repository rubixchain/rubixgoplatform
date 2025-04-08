package wallet

type MiningRecord struct {
	MiningID                 string `gorm:"column:mining_id"`
	MinedTokenID             string `gorm:"column:mined_token_id"`
	TokenLevelAndTokenNumber int    `gorm:"token_level_and_token_number"`
}

type MiningTransIDTokenIDPairs struct {
	TransactionID   string
	TransferTokenID string
}

func (w *Wallet) AddMiningRecords(miningRecord MiningRecord) error {
	err := w.s.Write(MiningRecordsTable, miningRecord)
	if err != nil {
		w.log.Error("failed to write mining records to mining records table")
	}
	return nil
}
