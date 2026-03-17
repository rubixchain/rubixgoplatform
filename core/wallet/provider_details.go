package wallet

import (
	"database/sql"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// GetProviderDetails retrieves the provider details for a specific token hash
// from the ipfs_providers table (consolidated from the former token_provider_map).
func (w *Wallet) GetProviderDetails(tokenHash string) (*model.TokenProviderMap, error) {
	var tpm model.TokenProviderMap
	var operation sql.NullString
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT cid, did, role, operation, transaction_id, initiator, owner, token_value
		 FROM ipfs_providers WHERE cid = $1 ORDER BY created_at DESC LIMIT 1`, tokenHash).
		Scan(&tpm.TokenHash, &tpm.DID, &tpm.Role, &operation,
			&tpm.TransactionID, &tpm.Initiator, &tpm.Owner, &tpm.TokenValue)
	if err != nil {
		return nil, fmt.Errorf("no provider details for token %s: %w", tokenHash, err)
	}
	// Map operation string back to FuncID for API compatibility
	if operation.Valid {
		switch operation.String {
		case constants.IPFSProviderOpAdd:
			tpm.FuncID = constants.TokenProviderFunc_Add
		case constants.IPFSProviderOpPin:
			tpm.FuncID = constants.TokenProviderFunc_Pin
		}
	}
	return &tpm, nil
}
