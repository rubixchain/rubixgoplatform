package core

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) GetDIDAccess(req *model.GetDIDAccess) *model.DIDAccessResponse {
	resp := &model.DIDAccessResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	dt, err := c.w.GetDID(req.DID)
	if err != nil {
		c.log.Error("DID does not exist", "err", err)
		resp.Message = "DID does not exist"
		return resp
	}
	if dt.Type == did.BasicDIDMode || dt.Type == did.ChildDIDMode {
		if !c.checkPassword(req.DID, req.Password) {
			resp.Message = "Password does not match"
			return resp
		}
	} else {
		_, ok := c.ValidateDIDToken(req.Token, setup.ChanllegeTokenType, req.DID)
		if !ok {
			resp.Message = "Invalid token"
			return resp
		}
		dc := did.InitDIDBasic(req.DID, c.didDir, nil)
		ok, err := dc.PvtVerify([]byte(req.Token), req.Signature)
		if err != nil {
			c.log.Error("Failed to verify DID signature", "err", err)
			resp.Message = "Failed to verify DID signature"
			return resp
		}
		if !ok {
			resp.Message = "Invalid signature"
			return resp
		}
	}
	expiresAt := time.Now().Add(time.Minute * 10)
	tkn := c.generateDIDToken(setup.AccessTokenType, req.DID, dt.RootDID == 1, expiresAt)
	resp.Status = true
	resp.Message = "Access granted"
	resp.Token = tkn
	return resp
}

func (c *Core) GetDIDChallenge(d string) *model.DIDAccessResponse {
	expiresAt := time.Now().Add(time.Minute * 1)
	return &model.DIDAccessResponse{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Challenge generated",
		},
		Token: c.generateDIDToken(setup.ChanllegeTokenType, d, false, expiresAt),
	}
}

func (c *Core) checkPassword(didStr string, pwd string) bool {
	privKey, err := ioutil.ReadFile(util.SanitizeDirPath(c.didDir) + didStr + "/" + did.PvtKeyFileName)
	if err != nil {
		c.log.Error("Private ket file does not exist", "did", didStr)
		return false
	}
	_, _, err = crypto.DecodeKeyPair(pwd, privKey, nil)
	if err != nil {
		c.log.Error("Invalid password", "did", didStr)
		return false
	}
	return true
}

func (c *Core) CreateDID(didCreate *did.DIDCreate) (string, error) {
	if didCreate.RootDID && didCreate.Type != did.BasicDIDMode {
		c.log.Error("only basic mode is allowed for root did")
		return "", fmt.Errorf("only basic mode is allowed for root did")
	}
	if didCreate.RootDID && c.w.IsRootDIDExist() {
		c.log.Error("root did is already exist")
		return "", fmt.Errorf("root did is already exist")
	}
	did, err := c.d.CreateDID(didCreate)
	if err != nil {
		return "", err
	}
	if didCreate.Dir == "" {
		didCreate.Dir = did
	}
	dt := wallet.DIDType{
		DID:    did,
		DIDDir: didCreate.Dir,
		Type:   didCreate.Type,
		Config: didCreate.Config,
	}
	if didCreate.RootDID {
		dt.RootDID = 1
	}
	err = c.w.CreateDID(&dt)
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
	dt, err := c.w.GetDIDs(dir)
	if err != nil {
		return nil
	}
	return dt
}

func (c *Core) IsDIDExist(dir string, did string) bool {
	_, err := c.w.GetDIDDir(dir, did)
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
	err = c.w.CreateDID(&dt)
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
