package core

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/util"
)

// Struct to match the API response
type APIResponse struct {
	Message string  `json:"message"`
	Data    DIDInfo `json:"data"`
}

type DIDInfo struct {
	UserDID string `json:"user_did"`
	DIDType string `json:"did_type"`
	PeerID  string `json:"peer_id"`
}

func (c *Core) GetPeerFromExplorer(didStr string) (*wallet.DIDPeerMap, error) {
	c.log.Debug("Fetching peer from explorer", "did", didStr)

	url := "https://rexplorer.azurewebsites.net/api/user/get-did-info/" + didStr
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to request explorer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("explorer returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read explorer response: %w", err)
	}

	c.log.Debug("Explorer raw response", "body", string(body))

	// First, parse basic structure
	var genericResp struct {
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &genericResp); err != nil {
		return nil, fmt.Errorf("failed to parse explorer response: %w", err)
	}

	if strings.Contains(genericResp.Message, "Deployer not found") {
		c.log.Error("Deployer not found for DID", "did", didStr)
		return nil, fmt.Errorf("peer not found in explorer for DID: %s", didStr)
	}

	// Parse full response
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse DID data: %w", err)
	}

	c.log.Debug("Explorer response parsed", "userDID", apiResp.Data.UserDID, "peerID", apiResp.Data.PeerID, "didType", apiResp.Data.DIDType)

	// Fetch DID content to local node
	if err := c.FetchDID(apiResp.Data.UserDID); err != nil {
		c.log.Error("Failed to fetch DID", "did", apiResp.Data.UserDID, "err", err)
		return nil, fmt.Errorf("failed to fetch DID from network: %w", err)
	}

	// Determine if DID has .png file to identify BasicDID
	hasPNG := false
	didDir := filepath.Join(c.didDir, apiResp.Data.UserDID)
	if files, err := os.ReadDir(didDir); err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".png") {
				hasPNG = true
				break
			}
		}
	} else {
		c.log.Warn("Failed to scan DID directory", "path", didDir, "err", err)
	}

	// Resolve DID type
	var resolvedType int
	switch apiResp.Data.DIDType {
	case "BIP39":
		resolvedType = did.LiteDIDMode
	default:
		if hasPNG {
			resolvedType = did.BasicDIDMode
		} else {
			resolvedType = did.LiteDIDMode // fallback default
		}
	}

	peerInfo := &wallet.DIDPeerMap{
		DID:     apiResp.Data.UserDID,
		PeerID:  apiResp.Data.PeerID,
		DIDType: &resolvedType,
	}

	// Add peer to table (upsert logic should be inside AddPeerDetails)
	if err := c.AddPeerDetails(*peerInfo); err != nil {
		c.log.Error("Failed to add peer details to table", "did", peerInfo.DID, "err", err)
		// Return peerInfo anyway, in case caller can proceed
		return peerInfo, nil
	}

	return peerInfo, nil
}

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
	} else if dt.Type == did.LiteDIDMode {
		_, ok := c.ValidateDIDToken(req.Token, setup.ChanllegeTokenType, req.DID)
		if !ok {
			resp.Message = "Invalid token"
			return resp
		}
		dc := did.InitDIDLite(req.DID, c.didDir, nil)
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
	newDID := &ExplorerDID{
		PeerID:  c.peerID,
		DID:     did,
		Balance: 0,
		DIDType: didCreate.Type,
	}
	c.ec.ExplorerUserCreate(newDID)
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
	newDID := &ExplorerDID{
		PeerID:  c.peerID,
		DID:     ds,
		DIDType: dc.Type,
	}
	c.ec.ExplorerUserCreate(newDID)
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
	c.UpdateUserInfo([]string{did}) //Updating the balance
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

	dt, err := c.w.GetDID(did)
	if err != nil {
		return fmt.Errorf("DID does not exist")
	}
	pm := &PeerMap{
		PeerID:    c.peerID,
		DID:       did,
		Signature: sig,
		Time:      t,
		DIDType:   dt.Type,
	}
	err = c.publishPeerMap(pm)
	if err != nil {
		c.log.Error("Register DID, failed to publish peer did map", "err", err)
		return err
	}
	return nil
}

// CreateDIDFromPubKey creates a DID from the provided public key
func (c *Core) CreateDIDFromPubKey(didCreate *did.DIDCreate, pubKey string) (string, error) {
	if didCreate.RootDID && didCreate.Type != did.BasicDIDMode {
		c.log.Error("only basic mode is allowed for root did")
		return "", fmt.Errorf("only basic mode is allowed for root did")
	}
	if didCreate.RootDID && c.w.IsRootDIDExist() {
		c.log.Error("root did is already exist")
		return "", fmt.Errorf("root did is already exist")
	}

	// pass public key and other requirements (did type) to create did for the
	// BIP wallet with corresponding public key
	did, err := c.d.CreateDIDFromPubKey(didCreate, pubKey)
	if err != nil {
		return "", err
	}
	if c.w.IsDIDExist(did) {
		return did, nil
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

	//store the created did in database
	err = c.w.CreateDID(&dt)
	if err != nil {
		c.log.Error("Failed to create did in the wallet", "err", err)
		return "", err
	}

	newDID := &ExplorerDID{
		PeerID:  c.peerID,
		DID:     did,
		Balance: 0,
		DIDType: didCreate.Type,
	}
	c.ec.ExplorerUserCreate(newDID)
	return did, nil
}

// This function, GetPeerDIDInfo, retrieves information about a peer's DID (Decentralized Identifier)
// from various sources, including the local database, an explorer, or directly from the peer.
// It returns a wallet.DIDPeerMap containing the peer's DID, Peer ID, and DID type,
// or an error if the information cannot be found.
func (c *Core) GetPeerDIDInfo(didStr string) (*wallet.DIDPeerMap, error) {
	c.log.Debug("Resolving peer info", "did", didStr)

	// 1. Try peer table first
	peerID := c.w.GetPeerID(didStr)
	didType, _ := c.w.GetPeerDIDType(didStr)

	if peerID != "" && didType != -1 {
		c.log.Debug("Found peer info in local storage", "peerID", peerID, "didType", didType)
		return &wallet.DIDPeerMap{
			DID:     didStr,
			PeerID:  peerID,
			DIDType: &didType,
		}, nil
	}

	// 2. If missing, try DID table
	if didType == -1 {
		if didInfo, err := c.w.GetDID(didStr); err == nil {
			didType = didInfo.Type
		}
	}

	// If peerID still missing, try resolving (via explorer or peer fetch)
	if peerID == "" {
		if !c.testNet {
			peerInfo, err := c.GetPeerFromExplorer(didStr)
			if err != nil {
				return nil, fmt.Errorf("explorer lookup failed: %w", err)
			}
			return peerInfo, nil
		}

		// Testnet: resolve from peer directly
		p, err := c.getPeer(didStr)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to peer: %w", err)
		}
		defer p.Close()

		details, err := c.GetPeerInfo(p, didStr)
		if err != nil {
			return nil, fmt.Errorf("failed to get peer info: %w", err)
		}

		peerInfo := &wallet.DIDPeerMap{
			DID:     didStr,
			PeerID:  p.GetPeerID(),
			DIDType: details.PeerInfo.DIDType,
		}
		// Save resolved info
		if err := c.AddPeerDetails(*peerInfo); err != nil {
			return nil, err
		}
		return peerInfo, nil
	}

	// PeerID exists, but no DIDType — fetch from peer or explorer
	if didType == -1 {
		var peerInfo *wallet.DIDPeerMap
		if !c.testNet {
			peerInfo, err := c.GetPeerFromExplorer(didStr)
			if err != nil {
				return nil, fmt.Errorf("explorer fetch failed: %w", err)
			}
			return peerInfo, nil
		}

		p, err := c.getPeer(peerID + "." + didStr)
		if err != nil {
			return nil, fmt.Errorf("peer connection failed: %w", err)
		}
		defer p.Close()

		peerDetails, err := c.GetPeerInfo(p, didStr)
		if err != nil {
			return nil, fmt.Errorf("peer info fetch failed: %w", err)
		}

		peerInfo = &wallet.DIDPeerMap{
			DID:     didStr,
			PeerID:  peerID,
			DIDType: peerDetails.PeerInfo.DIDType,
		}
		if err := c.AddPeerDetails(*peerInfo); err != nil {
			return nil, err
		}
		return peerInfo, nil
	}

	return &wallet.DIDPeerMap{
		DID:     didStr,
		PeerID:  peerID,
		DIDType: &didType,
	}, nil
}
