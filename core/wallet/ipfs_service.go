package wallet

import (
	"io"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// modified pin method that pins token and update in DB with role of the machine pinning
// If skipProviderDetails is true, do not call AddProviderDetails (for batch flows)
func (w *Wallet) Pin(hash string, role int, did string, transactionId string, initiator string, owner string, tokenValue float64, skipProviderDetails ...bool) (bool, error) {
	err := w.ipfsOps.Pin(hash)
	if err != nil {
		w.log.Error("Failed to pin token", "hash", hash, "error", err)
		return false, err
	}
	if len(skipProviderDetails) > 0 && skipProviderDetails[0] {
		return true, nil
	}
	err = w.AddProviderDetails(model.TokenProviderMap{TokenHash: hash, Role: role, DID: did, FuncID: constants.ProviderFunc_Pin, TransactionID: transactionId, Initiator: initiator, Owner: owner, TokenValue: tokenValue})
	if err != nil {
		w.log.Info("Error addding provider details to DB", "error", err)
		return false, err
	} else {
		w.log.Info("Provider details added to DB as pin -", " hash ", hash, " transactionID ", transactionId)
	}
	return true, nil
}

// modifeied unpin method that unpins token and deltes the entry
func (w *Wallet) UnPin(hash string, role int, did string) (bool, error) {
	err := w.ipfsOps.Unpin(hash)
	if err != nil {
		w.log.Error("Failed to unpin token", "hash", hash, "error", err)
		return false, err
	}
	err = w.RemoveProviderDetails(hash, did)
	if err != nil {
		w.log.Info("Error removing provider details to DB", "error", err)
		return false, err
	}
	return true, nil
}

func (w *Wallet) Cat(hash string, role int, did string) (string, error) {
	data1, err := w.ipfsOps.Cat(hash)
	if err != nil {
		w.log.Error("Error fetching details from ipfs", "error", err)
		return "", err
	}
	result, err := io.ReadAll(data1)
	if err != nil {
		w.log.Error("Error formatting ipfs content", "error", err)
		return "", err
	}
	err1 := w.AddProviderDetails(model.TokenProviderMap{TokenHash: hash, Role: role, DID: did, FuncID: constants.ProviderFunc_Cat})
	if err1 != nil {
		w.log.Info("Error addding provider details to DB", "error", err)
		return "", err
	} else {
		w.log.Info("Provider details added to DB as cat -", " hash ", hash)
	}
	return string(result), nil
}

func (w *Wallet) Get(hash string, did string, role int, path string) error {
	err := w.ipfsOps.Get(hash, path)
	if err != nil {
		w.log.Error("Error while getting file from ipfs", "error", err)
		return err
	}
	err = w.AddProviderDetails(model.TokenProviderMap{TokenHash: hash, Role: role, DID: did, FuncID: constants.ProviderFunc_Get})
	if err != nil {
		w.log.Info("Error addding provider details to DB", "error", err)
		//return err
	} else {
		w.log.Info("Provider details added to DB as get -", " hash ", hash)
	}
	return err
}

func (w *Wallet) Add(r io.Reader, did string, role int, skipProvider ...bool) (string, error) {
	result, err := w.ipfsOps.Add(r)
	if err != nil {
		w.log.Error("Error adding file to ipfs", "error", err)
		return "", err
	}

	if len(skipProvider) == 0 || !skipProvider[0] {
		err = w.AddProviderDetails(model.TokenProviderMap{TokenHash: result, Role: role, DID: did, FuncID: constants.ProviderFunc_Add})
		if err != nil {
			w.log.Error("Error adding provider details", "error", err)
			return "", err
		} else {
			w.log.Info("Provider details added to DB as Add -", " Token ", result)
		}
	}
	return result, err
}

// AddWithProviderMap adds to IPFS and returns the hash and TokenProviderMap for later batching
func (w *Wallet) AddWithProviderMap(r io.Reader, did string, role int) (string, model.TokenProviderMap, error) {
	result, err := w.ipfsOps.Add(r)
	if err != nil {
		w.log.Error("Error adding file to ipfs", "error", err)
		return "", model.TokenProviderMap{}, err
	}
	tpm := model.TokenProviderMap{
		TokenHash: result,
		Role:      role,
		DID:       did,
		FuncID:    constants.ProviderFunc_Add,
	}
	return result, tpm, nil
}
