package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/types/models"
)


// addProtocolValuesToLookupTables ensures that the lookup tables for DID algo, token statuses, and token types 
// are populated with the protocol-defined values (refer types/models/lookup.go).
func (w *Wallet) addProtocolValuesToLookupTables() error {
	// DID Algorithms
	for _, algo := range models.DidAlgoTypes {
		if _, err := w.db.Pool().Exec(w.Ctx,
			`INSERT INTO did_algo (name, is_active)
			 SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM did_algo WHERE name = $1)`,
			algo.Name, algo.IsActive,
		); err != nil {
			return fmt.Errorf("unable to add protocol values to did_algo %q: %w", algo.Name, err)
		}
	}

	// Token Statuses
	for _, status := range models.TokenStatusTypes {
		if _, err := w.db.Pool().Exec(w.Ctx,
			`INSERT INTO token_status (name, is_active)
			 SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM token_status WHERE name = $1)`,
			status.Name, status.IsActive,
		); err != nil {
			return fmt.Errorf("unable to add protocol values to token_status %q: %w", status.Name, err)
		}
	}

	// Token Types
	for _, t := range models.TokenTypeTypes {
		if _, err := w.db.Pool().Exec(w.Ctx,
			`INSERT INTO token_type (name, is_active)
			 SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM token_type WHERE name = $1)`,
			t.Name, t.IsActive,
		); err != nil {
			return fmt.Errorf("unable to add protocol values to token_type %q: %w", t.Name, err)
		}
	}

	return nil
}
