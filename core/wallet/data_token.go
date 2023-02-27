package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/block"
)

type DataToken struct {
	TokenID      string `gorm:"column:token_id;primary_key"`
	DID          string `gorm:"column:did"`
	CommitterDID string `gorm:"column:commiter_did"`
	TokenStatus  int    `gorm:"column:token_status;"`
}

// CreateDataToken write data token into db
func (w *Wallet) CreateDataToken(dt *DataToken) error {
	err := w.s.Write(DataTokenStorage, dt)
	if err != nil {
		w.log.Error("Failed to write data token", "err", err)
		return err
	}
	return nil
}

// AddDataTokenBlock will add token chain to db
func (w *Wallet) AddDataTokenBlock(t string, b *block.Block) error {
	return w.addBlock(DataTokenType, t, b)
}

func (w *Wallet) GetDataToken(did string) ([]DataToken, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var dts []DataToken
	err := w.s.Read(DataTokenStorage, &dts, "commiter_did=? AND token_status=?", did, TokenIsFree)
	if err != nil {
		return nil, err
	}
	if len(dts) == 0 {
		return nil, fmt.Errorf("no data token is available")
	}
	return dts, nil
}
