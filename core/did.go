package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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
	// Construct the API URL
	fmt.Println("Fetching peer info from explorer for DID:", didStr)
	url := "https://rexplorer.azurewebsites.net/api/user/get-did-info/" + didStr

	// Make the HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-200 status: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Check for known error message
	var genericResp struct {
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &genericResp); err != nil {
		return nil, fmt.Errorf("failed to parse generic JSON: %v", err)
	}

	if strings.Contains(genericResp.Message, "Deployer not found") {
		fmt.Println("Deployer not found for DID:", didStr)
		return nil, fmt.Errorf("PeerID not found for DID: %s", didStr)
	}

	// Parse the JSON response
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	fmt.Println("API Response:", apiResp)
	userDID := apiResp.Data.UserDID

	// Fetch the DID
	if err := c.FetchDID(userDID); err != nil {
		fmt.Println("Failed to fetch DID:", err)
		return nil, fmt.Errorf("failed to fetch DID: %v", err)
	}

	// Check for .png files in the folder
	didDirPath := filepath.Join(c.didDir, userDID)
	hasPNG := false

	files, err := ioutil.ReadDir(didDirPath)
	if err != nil {
		fmt.Println("Failed to read DID directory:", err)
		return nil, fmt.Errorf("failed to read DID directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".png" {
			fmt.Println("Found PNG file:", file.Name())
			hasPNG = true
			break
		}
	}

	// Initialize DIDType with a pointer
	didType := -1
	peerInfo := &wallet.DIDPeerMap{
		DID:     apiResp.Data.UserDID,
		PeerID:  apiResp.Data.PeerID,
		DIDType: &didType,
	}

	// Determine DIDType based on conditions
	if apiResp.Data.DIDType == "BIP39" {
		*peerInfo.DIDType = did.LiteDIDMode
	} else if hasPNG {
		mode := did.BasicDIDMode
		peerInfo.DIDType = &mode
	}

	fmt.Println("PeerInfo:", peerInfo, "DIDType:", *peerInfo.DIDType)

	if peerInfo.DIDType == &didType {
		c.log.Error("DID type not found for ", didStr)
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

// GetPeerDIDInfo fetched peer info either from DIDTable (if in the same node), or DIDPeerTable, or explorer
// If did type is still not found then it connects the peer and fetches did type
// And it adds peer to DIDPeerTable, in case peer is not in the same node
// This function throws error in 3 cases :
// case-1 : if peer details not found anywhere, returns nil, and error;
// case-2 : if peerId not found anywhere but did type is in DB, retrns did type and error;
// case-3 : if failed to add peer info to DB, returns DIDPeerMap and error containing 'retry' msg
func (c *Core) GetPeerDIDInfo(didStr string) (*wallet.DIDPeerMap, error) {
	peerDIDInfo := &wallet.DIDPeerMap{
		DID: didStr,
	}
	// check if peer is in same node
	didInfo, err := c.w.GetDID(didStr)
	if err == nil {
		peerDIDInfo.DIDType = &didInfo.Type
		peerDIDInfo.PeerID = c.peerID
		return peerDIDInfo, nil
	}
	// if peer is in different node, fetch peer id from DIDPeerTable
	peerID := c.w.GetPeerID(didStr)
	if peerID == "" {
		if c.testNet {
			didType, _ := c.w.GetPeerDIDType(didStr)
			if didType == -1 {
				c.log.Error("missing peer details of peer ", didStr)
				return nil, err
			}
			peerDIDInfo.DIDType = &didType
			return peerDIDInfo, fmt.Errorf("failed to find peer ID, err : " + err.Error())
		}
		// if peer id not found in table, try to fetch from explorer for mainnet RBTs
		peerDIDInfo, err = c.GetPeerFromExplorer(didStr)
		if peerDIDInfo != nil {
			c.log.Debug("PeerDIDInfo from explorer:", peerDIDInfo)
			c.AddPeerDetails(*peerDIDInfo)
		}
		if err != nil {
			c.log.Error("failed to fetch peer Id from explorer for ", didStr, "err", err)

			// if peer id not found in explorer too, then return only did type, if it is in table
			didType, _ := c.w.GetPeerDIDType(didStr)
			if didType == -1 {
				return nil, err
			}
			peerDIDInfo.DIDType = &didType
			return peerDIDInfo, fmt.Errorf("failed find peer ID, err : " + err.Error())
		}
	} else {
		peerDIDInfo.PeerID = peerID
	}

	//if did type is not fetched yet or is incorrect, then try to fetch it from db or from the peer itself
	if peerDIDInfo.DIDType == nil || *peerDIDInfo.DIDType == -1 {
		if !c.testNet {
			peerDIDInfo, err = c.GetPeerFromExplorer(didStr)
			if err != nil && *peerDIDInfo.DIDType != -1 {
				c.log.Error("failed to fetch peer details from explorer for ", didStr, "err", err)

			}
		} else {
			didType, _ := c.w.GetPeerDIDType(didStr)
			if didType == -1 {
				c.log.Debug("Connecting with peer to get DID type of peer did", didStr)
				p, err := c.getPeer(didStr + "." + peerDIDInfo.PeerID)
				if err != nil {
					c.log.Error("could not connect with peer to fetch did type, error ", err)
					return peerDIDInfo, nil
				}
				defer p.Close()
				peerDetails, err := c.GetPeerInfo(p, didStr)
				if err != nil {
					c.log.Error("failed to fetch did type from peer ", didStr, "err", err)
					return peerDIDInfo, nil
				}
				peerDIDInfo.DIDType = peerDetails.PeerInfo.DIDType
			} else {
				peerDIDInfo.DIDType = &didType
			}
		}

	}

	// add peer to DIDPeerTable, if peer is in different node
	if peerDIDInfo.PeerID != c.peerID {
		err = c.AddPeerDetails(*peerDIDInfo)
		if err != nil {
			c.log.Error("failed to add peer to DIDPeerTable, err ", err)
			return peerDIDInfo, fmt.Errorf("failed to add peer info to table, please retry adding")
		}
	}
	return peerDIDInfo, nil
}
