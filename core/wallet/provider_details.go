package wallet

// struct definition for Mapping token and reason the did is a provider
type TokenProviderMap struct {
	Token         string  `gorm:"column:token;primaryKey"`
	DID           string  `gorm:"column:did"`
	FuncID        int     `gorm:"column:func_id"`
	Role          int     `gorm:"column:role"`
	TransactionID string  `gorm:"column:transaction_id"`
	Sender        string  `gorm:"column:sender"`
	Receiver      string  `gorm:"column:receiver"`
	TokenValue    float64 `gorm:"column:token_value"`
}

// Method takes token hash as input and returns the Provider details
func (w *Wallet) GetProviderDetails(token string) (*TokenProviderMap, error) {
	var tokenMap TokenProviderMap
	err := w.s.Read(TokenProvider, &tokenMap, "token=?", token)
	if err != nil {
		if err.Error() == "no records found" {
			//w.log.Debug("Data Not avilable in DB")
			return &tokenMap, err
		} else {
			w.log.Error("Error fetching details from DB", "error", err)
			return &tokenMap, err
		}
	}
	return &tokenMap, nil
}

// Method to add provider details to DB during ipfs ops
// checks if entry exist for token,did either write or updates

func (w *Wallet) AddProviderDetails(tokenProviderMap TokenProviderMap) error {
	var tpm TokenProviderMap
	err := w.s.Read(TokenProvider, &tpm, "token=?", tokenProviderMap.Token)
	if err != nil || tpm.Token == "" {
		w.log.Info("Token Details not found: Creating new Record")
		// create new entry
		return w.s.Write(TokenProvider, tokenProviderMap)
	}
	return w.s.Update(TokenProvider, tokenProviderMap, "token=?", tokenProviderMap.Token)
}

// Method deletes entry ffrom DB during unpin op
func (w *Wallet) RemoveProviderDetails(token string, did string) error {
	return w.s.Delete(TokenProvider, nil, "did=? AND token=?", did, token)
}
