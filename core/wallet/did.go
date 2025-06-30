package wallet

import (
	"encoding/hex"
	"fmt"

	"github.com/ipfs/go-cid"
)

type DIDType struct {
	DID     string `gorm:"column:did;primaryKey"`
	Type    int    `gorm:"column:type"`
	DIDDir  string `gorm:"column:did_dir"`
	RootDID int    `gorm:"column:root_did"`
	Config  string `gorm:"column:config"`
}

type DIDPeerMap struct {
	DID         string `gorm:"column:did;primaryKey"`
	DIDType     *int   `gorm:"column:did_type"`
	PeerID      string `gorm:"column:peer_id"`
	DIDLastChar string `gorm:"column:did_last_char"`
}

func (w *Wallet) IsRootDIDExist() bool {
	var dt DIDType
	err := w.s.Read(DIDStorage, &dt, "root_did =?", 1)
	if err != nil {
		return false
	}
	return dt.RootDID == 1
}

func (w *Wallet) CreateDID(dt *DIDType) error {
	err := w.s.Write(DIDStorage, &dt)
	if err != nil {
		w.log.Error("Failed to create DID", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetAllDIDs() ([]DIDType, error) {
	var dt []DIDType
	err := w.s.Read(DIDStorage, &dt, "did!=?", "")
	if err != nil {
		w.log.Error("Failed to get DID", "err", err)
		return nil, err
	}
	return dt, nil
}

func (w *Wallet) GetDIDs(dir string) ([]DIDType, error) {
	var dt []DIDType
	err := w.s.Read(DIDStorage, &dt, "did_dir=?", dir)
	if err != nil {
		w.log.Error("Failed to get DID", "err", err)
		return nil, err
	}
	return dt, nil
}

func (w *Wallet) GetDIDDir(dir string, did string) (*DIDType, error) {
	var dt DIDType

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

func (w *Wallet) GetDID(did string) (*DIDType, error) {
	var dt DIDType
	err := w.s.Read(DIDStorage, &dt, "did=?", did)
	if err != nil {
		w.log.Error("Failed to get DID", "err", err)
		return nil, err
	}
	return &dt, nil
}

func (w *Wallet) IsDIDExist(did string) bool {
	var dt DIDType
	err := w.s.Read(DIDStorage, &dt, "did=?", did)
	if err != nil {
		w.log.Error("DID does nto exist", "did", did)
		return false
	}
	return true
}

func (w *Wallet) AddDIDPeerMap(did string, peerID string, didType int) error {
	lastChar, err := w.GetLastChar(did)
	if err != nil {
		return err
	}
	var dm DIDPeerMap
	err = w.s.Read(DIDStorage, &dm, "did=?", did)
	if err == nil {
		return nil
	}

	err = w.s.Read(DIDPeerStorage, &dm, "did=?", did)
	if err != nil {
		dm.DID = did
		dm.PeerID = peerID
		dm.DIDLastChar = lastChar
		dm.DIDType = &didType
		return w.s.Write(DIDPeerStorage, &dm)
	}
	if dm.PeerID != peerID {
		dm.PeerID = peerID
		return w.s.Update(DIDPeerStorage, &dm, "did=?", did)
	}
	if dm.DIDType == nil {
		dm.DIDType = &didType
		return w.s.Update(DIDPeerStorage, &dm, "did=?", did)
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

// Fetches did type of the given did from PeerDIDTable
func (w *Wallet) GetPeerDIDType(did string) (int, error) {
	var dm DIDPeerMap
	err := w.s.Read(DIDPeerStorage, &dm, "did=?", did)
	if err != nil {
		return -1, err
	}
	if dm.DIDType == nil {
		return -1, nil
	}

	return *dm.DIDType, nil
}

// Updates did type of the given did in PeerDIDTable
func (w *Wallet) UpdatePeerDIDType(did string, didtype int) (bool, error) {
	var dm DIDPeerMap
	err := w.s.Read(DIDPeerStorage, &dm, "did=?", did)
	if err != nil {
		w.log.Error("couldn't read from peer did table UpdatePeerDIDType", "err", err)
		return false, err
	}

	dm.DIDType = &didtype

	err1 := w.s.Update(DIDPeerStorage, &dm, "did=?", did)
	if err1 != nil {
		w.log.Error("couldn't update did type in peer did table for:", did)
		return false, err1
	}
	return true, nil
}
