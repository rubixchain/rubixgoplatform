package wallet

import (
	"fmt"
)

type DataToken struct {
	TokenID      string `gorm:"column:token_id;primaryKey" json:"token_id"`
	DID          string `gorm:"column:did" json:"did"`
	CommitterDID string `gorm:"column:commiter_did" json:"comiter_did"`
	BatchID      string `gorm:"column:batch_id" json:"batch_id"`
	TokenStatus  int    `gorm:"column:token_status;" json:"token_status"`
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

func (w *Wallet) GetAllDataTokens(did string) ([]DataToken, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var dts []DataToken
	err := w.s.Read(DataTokenStorage, &dts, "did=?", did)
	if err != nil {
		return nil, err
	}
	return dts, nil
}

func (w *Wallet) GetDataToken(batchID string) ([]DataToken, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var dts []DataToken
	err := w.s.Read(DataTokenStorage, &dts, "batch_id=? AND token_status=?", batchID, TokenIsFree)
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

func (w *Wallet) GetDataTokenByDID(did string) ([]DataToken, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var dts []DataToken
	err := w.s.Read(DataTokenStorage, &dts, "did=?", did)
	if err != nil {
		return nil, err
	}
	if len(dts) == 0 {
		return nil, fmt.Errorf("no data token is available to commit")
	}
	return dts, nil
}

func (w *Wallet) ReleaseDataToken(dts []DataToken) error {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	for i := range dts {
		dts[i].TokenStatus = TokenIsFree
		err := w.s.Update(DataTokenStorage, &dts[i], "token_id=?", dts[i].TokenID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) CommitDataToken(dts []DataToken) error {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	for i := range dts {
		dts[i].TokenStatus = TokenIsCommitted
		err := w.s.Update(DataTokenStorage, &dts[i], "token_id=?", dts[i].TokenID)
		if err != nil {
			return err
		}
	}
	return nil
}
