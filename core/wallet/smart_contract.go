package wallet

import (
	"fmt"
	"time"
)

type SmartContract struct {
	SmartContractHash string `gorm:"column:smart_contract_hash;primaryKey" json:"smart_contract_hash"`
	Deployer          string `gorm:"column:deployer" json:"deployer"`
	BinaryCodeHash    string `gorm:"column:binary_code_hash" json:"binary_code_hash"`
	RawCodeHash       string `gorm:"column:raw_code_hash" json:"raw_code_hash"`
	SchemaCodeHash    string `gorm:"column:schema_code_hash" json:"schema_code_hash"`
	ContractStatus    int    `gorm:"column:contract_status" json:"contract_status"`
}

type CallBackUrl struct {
	SmartContractHash string    `gorm:"column:smart_contract_hash;primaryKey" json:"smart_contract_hash"`
	CallBackUrl       string    `gorm:"column:callback_url" json:"callback_url"`
	CreatedAt         time.Time `gorm:"column:created_at" json:"created_at"`
}

func (w *Wallet) CreateSmartContractToken(sc *SmartContract) error {
	err := w.s.Write(SmartContractStorage, sc)
	if err != nil {
		w.log.Error("Failed to write smart contract token", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetSmartContractToken(smartContractToken string) ([]SmartContract, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var sc []SmartContract
	w.log.Debug("smart_contract_hash=?", smartContractToken)
	err := w.s.Read(SmartContractStorage, &sc, "smart_contract_hash=?", smartContractToken)
	if err != nil {
		w.log.Error("err", err)
		return nil, err
	}
	if len(sc) == 0 {
		return nil, fmt.Errorf("no smart contract token is available to commit")
	}

	for i := range sc {
		sc[i].ContractStatus = TokenIsGenerated
		err := w.s.Update(SmartContractStorage, &sc[i], "smart_contract_hash=?", sc[i].SmartContractHash)
		if err != nil {
			return nil, err
		}
	}

	return sc, nil
}

func (w *Wallet) GetSmartContractTokenByDeployer(did string) ([]SmartContract, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var sc []SmartContract
	err := w.s.Read(SmartContractStorage, &sc, "did=?", did)
	if err != nil {
		return nil, err
	}
	if len(sc) == 0 {
		return nil, fmt.Errorf("no data token is available to commit")
	}
	return sc, nil
}

func (w *Wallet) UpdateSmartContractStatus(smartContractToken string, tokenStatus int) error {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var sc SmartContract
	err := w.s.Read(SmartContractStorage, &sc, "smart_contract_hash=?", smartContractToken)
	if err != nil {
		w.log.Error("err", err)
		return err
	}
	sc.ContractStatus = tokenStatus
	err = w.s.Update(SmartContractStorage, &sc, "smart_contract_hash=?", smartContractToken)
	if err != nil {
		return err
	}
	return nil
}

// retrive state pin info if it exists
func (w *Wallet) GetStatePinnedInfo(token string) (*TokenProviderMap, error) {
	var tokenMap TokenProviderMap
	err := w.s.Read(TokenProvider, &tokenMap, "token=?", token)
	if err != nil {
		if err.Error() == "no records found" {
			//w.log.Debug("Data Not avilable in DB")
			return nil, nil
		} else {
			w.log.Error("Error fetching details from DB", "error", err)
			return nil, err
		}
	}
	return &tokenMap, nil
}

func (w *Wallet) WriteCallBackUrlToDB(input *CallBackUrl) error {
	err := w.s.Write(CallBackUrlStorage, input)
	if err != nil {
		w.log.Error("Failed to write smart contract token", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetSmartContractTokenUrl(smartcontracttoken string) (string, error) {
	var callback CallBackUrl
	err := w.s.Read(CallBackUrlStorage, &callback, "smart_contract_hash=?", smartcontracttoken)
	if err != nil {
		if err.Error() == "no records found" {
			w.log.Error("Smart contract not found in CallBackURL storage")
			return "", err
		} else {
			w.log.Error("Error fetching URL for Smart contract", "error:", err)
			return "", err
		}
	}
	url := callback.CallBackUrl
	return url, nil
}
