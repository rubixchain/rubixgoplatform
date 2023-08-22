package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) CreateDID(didCreate *did.DIDCreate) (string, error) {
	did, err := c.d.CreateDID(didCreate)
	if err != nil {
		return "", err
	}
	dt := wallet.DIDType{
		DID:    did,
		DIDDir: didCreate.Dir,
		Type:   didCreate.Type,
		Config: didCreate.Config,
	}
	err = c.W.CreateDID(&dt)
	if err != nil {
		c.log.Error("Failed to create did in the wallet", "err", err)
		return "", err
	}
	// exp := model.ExploreModel{
	// 	Cmd:     ExpDIDPeerMapCmd,
	// 	DIDList: []string{did},
	// 	PeerID:  c.peerID,
	// 	Message: "DID Created Successfully",
	// }
	// err = c.PublishExplorer(&exp)
	// if err != nil {
	// 	return "", err
	// }
	if !c.testNet {
		c.ec.ExplorerCreateDID(c.peerID, did)
	}
	return did, nil
}

func (c *Core) GetDIDs(dir string) []wallet.DIDType {
	dt, err := c.W.GetDIDs(dir)
	if err != nil {
		return nil
	}
	return dt
}

func (c *Core) IsDIDExist(dir string, did string) bool {
	_, err := c.W.GetDIDDir(dir, did)
	return err == nil
}

func (c *Core) AddDID(dc *did.DIDCreate) *model.BasicResponse {
	br := &model.BasicResponse{
		Status: false,
	}
	ds, err := c.d.MigrateDID(dc)
	if err != nil {
		br.Message = err.Error()
		return br
	}
	dt := wallet.DIDType{
		DID:    ds,
		DIDDir: dc.Dir,
		Type:   dc.Type,
		Config: dc.Config,
	}
	err = c.W.CreateDID(&dt)
	if err != nil {
		c.log.Error("Failed to create did in the wallet", "err", err)
		br.Message = err.Error()
		return br
	}
	c.ec.ExplorerCreateDID(c.peerID, ds)
	br.Status = true
	br.Message = "DID added successfully"
	br.Result = ds
	return br
}

func (c *Core) RegisterDID(reqID string, did string) {
	err := c.registerDID(reqID, did)
	br := model.BasicResponse{
		Status:  true,
		Message: "DID registered successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

func (c *Core) registerDID(reqID string, did string) error {
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return fmt.Errorf("DID is not exist")
	}
	t := time.Now().String()
	h := util.CalculateHashString(c.peerID+did+t, "SHA3-256")
	sig, err := dc.PvtSign([]byte(h))
	if err != nil {
		return fmt.Errorf("register did, failed to do signature")
	}
	pm := &PeerMap{
		PeerID:    c.peerID,
		DID:       did,
		Signature: sig,
		Time:      t,
	}
	err = c.publishPeerMap(pm)
	if err != nil {
		c.log.Error("Register DID, failed to publish peer did map", "err", err)
		return err
	}
	return nil
}
