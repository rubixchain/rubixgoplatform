package wallet

import (
	"encoding/hex"
	"fmt"

	"github.com/ipfs/go-cid"
)

type DID struct {
	DID     string `gorm:"column:did;primaryKey"`
	DIDDir  string `gorm:"column:did_dir"`
	RootDID int    `gorm:"column:root_did"`
	Config  string `gorm:"column:config"`
}

type DIDPeerMap struct {
	DID         string `gorm:"column:did;primaryKey"`
	PeerID      string `gorm:"column:peer_id"`
	DIDLastChar string `gorm:"column:did_last_char"`
}

func (w *Wallet) IsRootDIDExist() bool {
	var dt DID
	err := w.s.Read(DIDStorage, &dt, "root_did =?", 1)
	if err != nil {
		return false
	}
	return dt.RootDID == 1
}

func (w *Wallet) CreateDID(dt *DID) error {
	err := w.s.Write(DIDStorage, &dt)
	if err != nil {
		w.log.Error("Failed to create DID", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetAllDIDs() ([]DID, error) {
	var dt []DID
	err := w.s.Read(DIDStorage, &dt, "did!=?", "")
	if err != nil {
		w.log.Error("Failed to get DID", "err", err)
		return nil, err
	}
	return dt, nil
}

func (w *Wallet) GetDIDs(dir string) ([]DID, error) {
	var dt []DID
	err := w.s.Read(DIDStorage, &dt, "did_dir=?", dir)
	if err != nil {
		w.log.Error("Failed to get DID", "err", err)
		return nil, err
	}
	return dt, nil
}

func (w *Wallet) GetDIDDir(dir string, did string) (*DID, error) {
	var dt DID

	if dir == "" {
		err := w.s.Read(DIDStorage, &dt, "did=?", did)
		if err != nil {
			w.log.Error("DID does not exist", "did", did)
			return nil, err // Added missing return for error
		}
	} else {
		err := w.s.Read(DIDStorage, &dt, "did_dir=? AND did=?", dir, did)
		if err != nil {
			w.log.Error("Failed to get DID", "err", err)
			return nil, err // Return error here as well
		}
	}

	return &dt, nil
}

func (w *Wallet) GetDID(did string) (*DID, error) {
	var dt DID
	err := w.s.Read(DIDStorage, &dt, "did=?", did)
	if err != nil {
		w.log.Error("Failed to get DID", "err", err)
		return nil, err
	}
	return &dt, nil
}

func (w *Wallet) IsDIDExist(did string) bool {
	var dt DID
	err := w.s.Read(DIDStorage, &dt, "did=?", did)
	if err != nil {
		w.log.Error("DID does nto exist", "did", did)
		return false
	}
	return true
}

func (w *Wallet) RemoveDID(did string) error {
	err := w.s.Delete(DIDStorage, &DID{}, "did=?", did)
	if err != nil {
		errMsg := fmt.Sprintf("DID could not be removed from DIDTable, did : %v, err : %v", did, err)
		w.log.Error(errMsg)
		return fmt.Errorf("%v", errMsg)
	}
	return nil
}

func (w *Wallet) AddDIDPeerMap(did string, peerID string) error {
	lastChar, err := w.GetLastChar(did)
	if err != nil {
		return err
	}

	var existing DIDPeerMap

	//If DID exists in DIDStorage (authoritative), do nothing
	err = w.s.Read(DIDStorage, &existing, "did=?", did)
	if err == nil {
		return nil
	}

	//Try reading from DIDPeerStorage
	err = w.s.Read(DIDPeerStorage, &existing, "did=?", did)
	if err != nil {
		// Not found — insert new record
		newRecord := DIDPeerMap{
			DID:         did,
			PeerID:      peerID,
			DIDLastChar: lastChar,
		}
		return w.s.Write(DIDPeerStorage, &newRecord)
	}

	//Record exists — compare values
	samePeerID := existing.PeerID == peerID
	sameLastChar := existing.DIDLastChar == lastChar

	// If all match, nothing to update
	if samePeerID && sameLastChar {
		return nil
	}

	//Update changed fields
	existing.PeerID = peerID
	existing.DIDLastChar = lastChar

	return w.s.Update(DIDPeerStorage, &existing, "did=?", did)
}

// remove stale peer
func (w *Wallet) RemoveStalePeerDID(peerDID, peerId string) error {
	err := w.s.Delete(DIDPeerStorage, &DIDPeerMap{}, "did=? AND peer_id=?", peerDID, peerId)
	if err != nil {
		errMsg := fmt.Sprintf("peer-DID could not be removed from DIDPeerTable, peer-did : %v, err : %v", peerDID, err)
		w.log.Error(errMsg)
		return fmt.Errorf("%v", errMsg)
	}
	return nil
}

func (w *Wallet) AddDIDLastChar() error {
	var existingDIDPeer []DIDPeerMap
	err := w.s.Read(DIDPeerStorage, &existingDIDPeer, "did_last_char is NULL")
	if err != nil {
		return err
	}
	for _, dm := range existingDIDPeer {
		did := dm.DID
		lastChar, err := w.GetLastChar(did)
		if err != nil {
			continue
		}
		dm.DIDLastChar = lastChar
		err = w.s.Update(DIDPeerStorage, &dm, "did=?", did)
		w.log.Info("DID Peer table updated")
		if err != nil {
			w.log.Error("Unable to update DID Peer table.")
			return err
		}
	}
	return nil
}

func (w *Wallet) GetLastChar(did string) (string, error) {
	// Parse the did
	c, err := cid.Decode(did)
	if err != nil {
		w.log.Error(fmt.Sprintf("Failed to decode DID %v : %v", did, err))
		return "", err
	}
	multihashDigest := c.Hash()
	// Convert the multihash digest to hexadecimal - to compare with txnID
	hexDigest := hex.EncodeToString(multihashDigest)
	lastchar := string(hexDigest[len(hexDigest)-1])
	return lastchar, nil
}

func (w *Wallet) GetPeerID(did string) string {
	var dm DIDPeerMap
	err := w.s.Read(DIDPeerStorage, &dm, "did=?", did)
	if err != nil {
		w.log.Error("couldn't read from peer did table", "err", err)
		return ""
	}
	return dm.PeerID
}
