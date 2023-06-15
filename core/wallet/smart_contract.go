package wallet

import "fmt"

type SmartContract struct {
	SmartContractHash string `gorm:"column:smart_contract_hash;primaryKey" json:"smart_contract_id"`
	Deployer          string `gorm:"column:deployer" json:"deployer"`
	BinaryCodeHash    string `gorm:"column:binary_code_hash" json:"binary_code_hash"`
	RawCodeHash       string `gorm:"column:raw_code_hash" json:"raw_code_hash"`
	YamlCodeHash      string `gorm:"column:yaml_code_hash" json:"yaml_code_hash"`
	ContractStatus    int    `gorm:"column:contract_status" json:"contract_status"`
}

func (w *Wallet) CreateSmartContractToken(sc *SmartContract) error {
	err := w.s.Write(SmartContractStorage, sc)
	if err != nil {
		w.log.Error("Failed to write smart contract token", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetSmartContractToken(smartContractID string) ([]SmartContract, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var sc []SmartContract
	err := w.s.Read(SmartContractStorage, &sc, "smart_contract_hash=?", smartContractID)
	if err != nil {
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

func (w *Wallet) DeploySmartContract(sc []SmartContract) error {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	for i := range sc {
		if sc[i].ContractStatus != TokenIsGenerated {
			return fmt.Errorf("smart contract token is not generated")
		}
		sc[i].ContractStatus = TokenIsDeployed
		err := w.s.Update(SmartContractStorage, &sc[i], "smart_contract_id=?", sc[i].SmartContractHash)
		if err != nil {
			return err
		}

	}

	return nil
}
