package wallet

type Credit struct {
	DID    string `gorm:"column:did"`
	Credit string `gorm:"column:credit;size:4000"`
}

func (w *Wallet) StoreCredit(did string, credit string) error {
	c := &Credit{
		DID:    did,
		Credit: credit,
	}
	return w.S.Write(CreditStorage, c)
}

func (w *Wallet) GetCredit(did string) ([]string, error) {
	var c []Credit
	err := w.S.Read(CreditStorage, &c, "did=?", did)
	if err != nil {
		return nil, err
	}
	str := make([]string, 0)
	for i := range c {
		str = append(str, c[i].Credit)
	}
	return str, nil
}

func (w *Wallet) RemoveCredit(did string, credit []string) error {
	for _, c := range credit {
		err := w.S.Delete(CreditStorage, &c, "did=? AND credit=?", did, c)
		if err != nil {
			return err
		}
	}
	return nil
}
