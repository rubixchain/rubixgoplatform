package wallet

// struct definition for Mapping token and reason the did is a provider
type TokenProviderMap struct {
	Token  string `gorm:"column:token;primaryKey"`
	DID    string `gorm:"column:did"`
	FuncID int    `gorm:"column:func_id"`
	Role   int    `gorm:"column:role"`
}

// Method takes token hash as input and returns the Provider details
func (w *Wallet) GetProviderDetails(token string) (*TokenProviderMap, error) {
	var tokenMap TokenProviderMap
	err := w.s.Read(TokenProvider, &tokenMap, "token=?", token)
	if err != nil {
		if err.Error() == "no records found" {
			w.log.Debug("Data Not avilable in DB")
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
func (w *Wallet) AddProviderDetails(token string, did string, funId int, role int) error {
	var tpm TokenProviderMap
	err := w.s.Read(TokenProvider, &tpm, "token=?", token)
	if err != nil || tpm.Token == "" {
		tpm.Token = token
		tpm.DID = did
		tpm.FuncID = funId
		tpm.Role = role
		return w.s.Write(TokenProvider, &tpm)
	}
	tpm.DID = did
	tpm.FuncID = funId
	tpm.Role = role
	return w.s.Update(TokenProvider, &tpm, "token=?", token)
}

// Method deletes entry ffrom DB during unpin op
func (w *Wallet) RemoveProviderDetails(token string, did string) error {
	return w.s.Delete(TokenProvider, nil, "did=? AND token=?", did, token)
}
