package wallet

type DIDType struct {
	DID     string `gorm:"column:did;primaryKey"`
	Type    int    `gorm:"column:type"`
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
	err := w.s.Read(DIDStorage, &dm, "did=?", did)
	if err == nil {
		return nil
	}
	err = w.s.Read(DIDPeerStorage, &dm, "did=?", did)
	if err != nil {
		dm.DID = did
		dm.PeerID = peerID
		dm.DIDLastChar = lastChar
		return w.s.Write(DIDPeerStorage, &dm)
	}
	if dm.PeerID != peerID {
		dm.PeerID = peerID
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
		lastChar := string(did[len(did)-1])
		dm.DIDLastChar = lastChar
		err := w.s.Update(DIDPeerStorage, &dm, "did=?", did)
		w.log.Info("DID Peer table updated")
		if err != nil {
			w.log.Error("Unable to update DID Peer table.")
			return err
		}
	}
	return nil
}
func (w *Wallet) GetPeerID(did string) string {
	var dm DIDPeerMap
	err := w.s.Read(DIDPeerStorage, &dm, "did=?", did)
	if err != nil {
		return ""
	}
	return dm.PeerID
}
