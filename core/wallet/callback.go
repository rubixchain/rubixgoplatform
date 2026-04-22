package wallet

import (
	"fmt"
)

// RegisterCallbackURL inserts or updates a callback URL for a smart contract.
// Uses UPSERT (ON CONFLICT) to handle both new registrations and updates.
func (w *Wallet) RegisterCallbackURL(smartContractHash string, callbackURL string) error {
	_, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO call_back_urls (smart_contract_hash, callback_url, created_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())
		 ON CONFLICT (smart_contract_hash)
		 DO UPDATE SET callback_url = EXCLUDED.callback_url, updated_at = NOW()`,
		smartContractHash, callbackURL,
	)
	if err != nil {
		return fmt.Errorf("RegisterCallbackURL: failed to register callback URL for smart contract %s: %w", smartContractHash, err)
	}
	return nil
}

// GetCallbackURL retrieves the callback URL for a given smart contract hash.
func (w *Wallet) GetCallbackURL(smartContractHash string) (string, error) {
	var callbackURL string
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT callback_url FROM call_back_urls WHERE smart_contract_hash = $1`,
		smartContractHash,
	).Scan(&callbackURL)
	if err != nil {
		return "", fmt.Errorf("GetCallbackURL: failed to get callback URL for smart contract %s: %w", smartContractHash, err)
	}
	return callbackURL, nil
}

// DeleteCallbackURL removes the callback URL registration for a smart contract.
func (w *Wallet) DeleteCallbackURL(smartContractHash string) error {
	_, err := w.db.Pool().Exec(w.Ctx,
		`DELETE FROM call_back_urls WHERE smart_contract_hash = $1`,
		smartContractHash,
	)
	if err != nil {
		return fmt.Errorf("DeleteCallbackURL: failed to delete callback URL for smart contract %s: %w", smartContractHash, err)
	}
	return nil
}
