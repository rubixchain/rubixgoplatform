package wallet

import "fmt"

type DIDType struct {
	DID    string `gorm:"column:did;primaryKey"`
	Type   int    `gorm:"column:type"`
	DIDDir string `gorm:"column:did_dir"`
	Config string `gorm:"column:config"`
}

type DIDPeerMap struct {
	DID     string `gorm:"column:did;primaryKey"`
	PeerID  string `gorm:"column:peer_id"`
	DIDChar string `gorm:"column:did_char"`
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
	var dm DIDPeerMap
	err := w.s.Read(DIDPeerStorage, &dm, "did=?", did)
	if err != nil {
		dm.DID = did
		dm.PeerID = peerID
		dm.DIDChar = lastChar
		return w.s.Write(DIDPeerStorage, &dm)
	}

	dm.PeerID = peerID
	return w.s.Update(DIDPeerStorage, &dm, "did=?", did)
}

func (w *Wallet) AddDIDChar() error {
	var existingDIDPeer []DIDPeerMap
	err := w.s.Read(DIDPeerStorage, &existingDIDPeer, "did!=?", "")
	if err != nil {
		w.log.Error("Unable to read from DID Peer table", err)
		return err
	}
	for i := 0; i < len(existingDIDPeer); i++ {
		dm := existingDIDPeer[i]
		did := dm.DID
		lastChar := string(did[len(did)-1])
		dm.DIDChar = lastChar
		err := w.s.Update(DIDPeerStorage, &dm, "did=?", did)
		if err != nil {
			w.log.Error("Unable to update DIDChar table.")
			return err
		}
	}
	return nil
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
