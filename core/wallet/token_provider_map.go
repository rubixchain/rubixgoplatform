package wallet

import (
	"fmt"
	"io"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) Add(r io.Reader, did string, role int, skipProvider ...bool) (string, error) {
	result, err := w.ipfsOps.Add(r)
	if err != nil {
		w.log.Error("Error adding file to ipfs", "error", err)
		return "", err
	}

	if len(skipProvider) == 0 || !skipProvider[0] {
		err := w.AddProviderDetails(models.TokenProviderMap{
			Token: result, Role: constants.TokenProviderFunc_Add, DID: did, FuncID: constants.TokenProviderFunc_Add})
		if err != nil {
			w.log.Error("Error adding provider details", "error", err)
			return "", err
		} else {
			w.log.Info("Provider details added to DB as Add -", " Token ", result)
		}
	}
	return result, err
}

func (w *Wallet) Pin(hash string, role int, did string, transactionId string, sender string, receiver string, tokenValue float64, skipProviderDetails ...bool) (bool, error) {
	err := w.ipfsOps.Pin(hash)
	if err != nil {
		w.log.Error("Failed to pin token", "hash", hash, "error", err)
		return false, err
	}
	if len(skipProviderDetails) > 0 && skipProviderDetails[0] {
		return true, nil
	}
	err = w.AddProviderDetails(models.TokenProviderMap{Token: hash, Role: role, DID: did, FuncID: constants.TokenProviderFunc_Pin, TransactionID: transactionId, Sender: sender, Receiver: receiver, TokenValue: tokenValue})
	if err != nil {
		w.log.Info("Error addding provider details to DB", "error", err)
		return false, err
	} else {
		w.log.Info("Provider details added to DB as pin -", " hash ", hash, " transactionID ", transactionId)
	}
	return true, nil
}

func (w *Wallet) AddProviderDetails(tpm models.TokenProviderMap) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO token_provider_map(token, did, func_id, role, transaction_id, sender, receiver, token_value)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tpm.Token, tpm.DID, tpm.FuncID, tpm.Role, tpm.TransactionID, tpm.Sender, tpm.Receiver,
		tpm.TokenValue,
	); err != nil {
		return fmt.Errorf("unable to add token provider details for token %v, err: %v", tpm.Token, err)
	}

	return nil
}
