package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// addProtocolValuesToLookupTables ensures that the lookup tables for DID algo, token roles, and token types
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

	// Token Roles
	for _, status := range models.TokenRoleTypes {
		if _, err := w.db.Pool().Exec(w.Ctx,
			`INSERT INTO token_role (name, is_active)
			 SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM token_role WHERE name = $1)`,
			status.Name, status.IsActive,
		); err != nil {
			return fmt.Errorf("unable to add protocol values to token_role %q: %w", status.Name, err)
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

	if err := w.assertLookupIDsMatchProtocolOrder(); err != nil {
		return err
	}

	return nil
}

// assertLookupIDsMatchProtocolOrder verifies that the IDs Go computes
// positionally (idx+1 from the slices in types/models/lookup.go) match the IDs
// Postgres assigned via GENERATED ALWAYS AS IDENTITY. If they diverge, every
// token row is written with the wrong type or role and nothing errors, so
// failing to boot is preferable.
func (w *Wallet) assertLookupIDsMatchProtocolOrder() error {
	for _, t := range models.TokenTypeTypes {
		var dbID int
		if err := w.db.Pool().QueryRow(w.Ctx,
			`SELECT id FROM token_type WHERE name = $1`, t.Name,
		).Scan(&dbID); err != nil {
			return fmt.Errorf("lookup integrity: unable to read token_type id for %q: %w", t.Name, err)
		}
		if goID := models.GetTokenTypeID(t.Name); goID != dbID {
			return fmt.Errorf("lookup integrity: token_type %q is id %d in the database but %d in the protocol order (types/models/lookup.go); entries may only be appended", t.Name, dbID, goID)
		}
	}

	for _, r := range models.TokenRoleTypes {
		var dbID int
		if err := w.db.Pool().QueryRow(w.Ctx,
			`SELECT id FROM token_role WHERE name = $1`, r.Name,
		).Scan(&dbID); err != nil {
			return fmt.Errorf("lookup integrity: unable to read token_role id for %q: %w", r.Name, err)
		}
		if goID := models.GetTokenRoleID(r.Name); goID != dbID {
			return fmt.Errorf("lookup integrity: token_role %q is id %d in the database but %d in the protocol order (types/models/lookup.go); entries may only be appended", r.Name, dbID, goID)
		}
	}

	return nil
}

// GetDidAlgoIDByName returns the DB id for a DID algorithm name.
func (w *Wallet) GetDidAlgoIDByName(name string) (int64, error) {
	var id int64
	if err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT id FROM did_algo WHERE name = $1 AND is_active = true`,
		name,
	).Scan(&id); err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("GetDidAlgoIDByName: did algo %q not found in did_algo table", name)
		}
		return 0, fmt.Errorf("GetDidAlgoIDByName: failed to fetch did algo id for %q: %w", name, err)
	}

	return id, nil
}
