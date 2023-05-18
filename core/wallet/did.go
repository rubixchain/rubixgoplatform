package wallet

import "fmt"

type DIDType struct {
	DID    string `gorm:"column:did;primaryKey"`
	Type   int    `gorm:"column:type"`
	DIDDir string `gorm:"column:did_dir"`
	Config string `gorm:"column:config"`
}

type DIDPeerMap struct {
	DID    string `gorm:"column:did;primaryKey"`
	PeerID string `gorm:"column:peer_id"`
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
	err := w.s.Read(DIDStorage, &dt, "did_dir=? AND did=?", dir, did)
	if err != nil {
		w.log.Error("Failed to get DID", "err", err)
		return nil, err
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

func (w *Wallet) AddDIDPeerMap(did string, peerID string) error {
	lastChar := string(did[len(did)-1])
	tableName := fmt.Sprintf("DIDPeerTable_%s", lastChar)
	var dm DIDPeerMap
	err := w.s.Read(tableName, &dm, "did=?", did)
	if err != nil {
		dm.DID = did
		dm.PeerID = peerID
		return w.s.Write(tableName, &dm)
	}

	dm.PeerID = peerID
	return w.s.Update(tableName, &dm, "did=?", did)
}

func (w *Wallet) GetPeerID(did string) string {
	lastChar := string(did[len(did)-1])
	tableName := fmt.Sprintf("DIDPeerTable_%s", lastChar)
	var dm DIDPeerMap
	err := w.s.Read(tableName, &dm, "did=?", did)
	if err != nil {
		return ""
	}
	return dm.PeerID
}
