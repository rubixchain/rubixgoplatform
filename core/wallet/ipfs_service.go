package wallet

import (
	"io"
	"io/ioutil"
)

const (
	PinFunc int = iota + 1
	UnPinFunc
	CatFunc
	GetFunc
	AddFunc
)

const (
	OwnerRole int = iota + 1
	QuorumRole
	PrevSenderRole
	ReceiverRole
	ParentTokenLockRole
	DIDRole
	StakingRole
	PledgingRole
	QuorumPinRole
	QuorumUnpinRole
	ParentTokenPinByQuorumRole
)

// modified pin method that pins token and update in DB with role of the machine pinning
func (w *Wallet) Pin(hash string, role int, did string, transactionId string, sender string, receiver string, tokenValue float64) (bool, error) {
	w.ipfs.Pin(hash)
	err := w.AddProviderDetails(TokenProviderMap{Token: hash, Role: role, DID: did, FuncID: PinFunc, TransactionID: transactionId, Sender: sender, Receiver: receiver, TokenValue: tokenValue})
	if err != nil {
		w.log.Info("Error addding provider details to DB", "error", err)
		return false, err
	}
	return true, nil
}

// modifeied unpin method that unpins token and deltes the entry
func (w *Wallet) UnPin(hash string, role int, did string) (bool, error) {
	w.ipfs.Unpin(hash)
	err := w.RemoveProviderDetails(hash, did)
	if err != nil {
		w.log.Info("Error removing provider details to DB", "error", err)
		return false, err
	}
	return true, nil
}

func (w *Wallet) Cat(hash string, role int, did string) (string, error) {
	data1, err := w.ipfs.Cat(hash)
	if err != nil {
		w.log.Error("Error fetching details from ipfs", "error", err)
		return "", err
	}
	result, err := ioutil.ReadAll(data1)
	if err != nil {
		w.log.Error("Error formatting ipfs content", "error", err)
		return "", err
	}
	err1 := w.AddProviderDetails(TokenProviderMap{Token: hash, Role: role, DID: did, FuncID: CatFunc})
	if err1 != nil {
		w.log.Info("Error addding provider details to DB", "error", err)
		return "", err
	}
	return string(result), nil
}

func (w *Wallet) Get(hash string, did string, role int, path string) error {
	err := w.ipfs.Get(hash, path)
	if err != nil {
		w.log.Error("Error while getting file from ipfs", "error", err)
		return err
	}
	err = w.AddProviderDetails(TokenProviderMap{Token: hash, Role: role, DID: did, FuncID: GetFunc})
	return err
}

func (w *Wallet) Add(r io.Reader, did string, role int) (string, error) {
	result, err := w.ipfs.Add(r)
	if err != nil {
		w.log.Error("Error adding file to ipfs", "error", err)
		return "", err
	}
	err = w.AddProviderDetails(TokenProviderMap{Token: result, Role: role, DID: did, FuncID: AddFunc})
	if err != nil {
		w.log.Error("Error adding provider details", "error", err)
		return "", err
	}
	return result, err
}
