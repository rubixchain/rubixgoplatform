package wallet

type PledgePinsDetails struct {
	QuorumDID string `gorm:"column:date_time"`
	TokenID   string `gorm:"column:status"`
	EpochTime string `gorm:"column:epoch_time"`
}

func (w *Wallet) AddPins(pd *PledgePinsDetails) error {
	err := w.S.Write(PledgePinsStorage, pd)
	if err != nil {
		w.log.Error("Failed to store pins", "err", err)
		return err
	}
	return nil
}
