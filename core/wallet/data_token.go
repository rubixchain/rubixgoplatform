package wallet

import (
	"fmt"
)

type DataToken struct {
	TokenID      string `gorm:"column:token_id;primaryKey"`
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

func (w *Wallet) GetDataToken(did string) ([]DataToken, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var dts []DataToken
	err := w.s.Read(DataTokenStorage, &dts, "commiter_did=? AND token_status=?", did, TokenIsFree)
	if err != nil {
		return nil, err
	}
	if len(dts) == 0 {
		return nil, fmt.Errorf("no data token is available to commit")
	}
	for i := range dts {
		dts[i].TokenStatus = TokenIsLocked
		err := w.s.Update(DataTokenStorage, &dts[i], "token_id=?", dts[i].TokenID)
		if err != nil {
			return nil, err
		}
	}
	return dts, nil
}
