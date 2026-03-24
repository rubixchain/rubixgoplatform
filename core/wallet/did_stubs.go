package wallet

import "github.com/rubixchain/rubixgoplatform/types/models"

// DID is a legacy wallet type for DID storage via block-based paths.
// TODO(phase07): replace usages with models.DID from types/models.
type DID struct {
	DID     string
	DIDDir  string
	Config  string
	RootDID int
}

// GetDIDDir returns the directory path for the given DID.
func (w *Wallet) GetDIDDir(dir string, did string) (string, error) {
	// TODO(phase07): query DID directory from DB
	return "", nil
}

// CreateDID persists a DID record (legacy path).
func (w *Wallet) CreateDID(dt *DID) error {
	// TODO(phase07): implement via CreateOrUpdateDID with models.DID
	didModel := &models.DID{
		DID:    dt.DID,
		PeerID: "",
		Local:  true,
	}
	return w.CreateOrUpdateDID(didModel)
}

// IsRootDIDExist returns true if a root DID record exists in the DB.
func (w *Wallet) IsRootDIDExist() bool {
	// TODO(phase07): query DID table for root_did=1
	return false
}

// RemoveDID deletes a DID record from the DB.
func (w *Wallet) RemoveDID(did string) error {
	// TODO(phase07): delete from did_table where did=$1
	return nil
}
