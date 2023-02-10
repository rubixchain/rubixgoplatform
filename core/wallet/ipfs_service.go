package wallet

import (
	"io"
	"io/ioutil"
)

const (
	Pin   int = 1
	UnPin int = 2
	Cat   int = 3
	Get   int = 4
	Add   int = 5
)

const (
	Owner        int = 1
	Quorum       int = 2
	PrevSender   int = 3
	Receiver     int = 4
	ParentToken  int = 5
	Did          int = 6
	StakedToken  int = 7
	PledgedToken int = 8
)

var FunctionMap = map[int]string{
	1: "pin",
	2: "unpin",
	3: "cat",
	4: "get",
	5: "add",
}

var RoleMap = map[int]string{
	1: "owner",
	2: "quorum",
	3: "prevSender",
	4: "receiver",
	5: "parentTokenLock",
	6: "did",
	7: "staking",
	8: "pledging",
}

// modified pin method that pins token and update in DB with role of the machine pinning
func (w *Wallet) Pin(hash string, role int, did string) (bool, error) {
	w.ipfs.Pin(hash)
	err := w.AddProviderDetails(hash, did, Pin, role)
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
	err1 := w.AddProviderDetails(hash, did, Cat, role)
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
	err = w.AddProviderDetails(hash, did, Get, role)
	return err
}

func (w *Wallet) Add(r io.Reader, did string, role int) (string, error) {
	result, err := w.ipfs.Add(r)
	if err != nil {
		w.log.Error("Error adding file to ipfs", "error", err)
		return "", err
	}
	err = w.AddProviderDetails(result, did, Add, role)
	return result, nil
}
