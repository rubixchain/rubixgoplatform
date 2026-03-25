package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
)

// SmartContract is a legacy stub type for smart contract tokens.
type SmartContract struct {
	SmartContractHash string
	Deployer          string
	BinaryCodeHash    string
	RawCodeHash       string
	ContractStatus    int
}

// SmartContractContent is a legacy stub for smart contract content.
type SmartContractContent struct {
	SmartContractHash  string
	DeployerDID        string
	BinaryCodeFileName string
	RawCodeFileName    string
	BinaryCode         []byte
	RawCode            []byte
}

func (w *Wallet) CreateSmartContractToken(sc *SmartContract) error {
	_, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO tokens(token_id, token_value, token_status, did, transaction_id, token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
		 VALUES ($1, 0, $2, $3, '', '', (SELECT id FROM token_type WHERE name=$4), 0, 0, NOW(), NOW())
		 ON CONFLICT(token_id) DO UPDATE SET token_status=EXCLUDED.token_status, did=EXCLUDED.did, updated_at=NOW()`,
		sc.SmartContractHash, sc.ContractStatus, sc.Deployer, constants.TokenType_SmartContract,
	)
	if err != nil {
		return fmt.Errorf("CreateSmartContractToken: %w", err)
	}
	return nil
}

func (w *Wallet) AddSmartContractContentToPSQl(scc *SmartContractContent) error {
	// TODO(phase09): implement smart contract content storage
	return nil
}

// GetSmartContractTokenByDeployer returns all smart contract tokens deployed by the given DID.
// TODO(phase07): implement full PostgreSQL query against tokens table with token_type=smart_contract.
func (w *Wallet) GetSmartContractTokenByDeployer(did string) ([]SmartContract, error) {
	return nil, nil
}

// GetSmartContractToken returns smart contract token(s) matching the given token ID.
// TODO(phase07): implement full PostgreSQL query against tokens table.
func (w *Wallet) GetSmartContractToken(tokenID string) ([]SmartContract, error) {
	return nil, nil
}
