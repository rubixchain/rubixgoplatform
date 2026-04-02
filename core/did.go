package core

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// Struct to match the API response
type APIResponse struct {
	Message string  `json:"message"`
	Data    DIDInfo `json:"data"`
}

type DIDInfo struct {
	UserDID string `json:"user_did"`
	PeerID  string `json:"peer_id"`
}

func (c *Core) GetPeerFromExplorer(didStr string) (*models.DID, error) {
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

	c.log.Debug("Explorer response parsed", "userDID", apiResp.Data.UserDID, "peerID", apiResp.Data.PeerID)

	// Fetch DID content to local node
	if err := c.FetchDID(apiResp.Data.UserDID); err != nil {
		c.log.Error("Failed to fetch DID", "did", apiResp.Data.UserDID, "err", err)
		return nil, fmt.Errorf("failed to fetch DID from network: %w", err)
	}

	peerInfo := &models.DID{
		DID:    apiResp.Data.UserDID,
		PeerID: apiResp.Data.PeerID,
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
	_, ok := c.ValidateDIDToken(req.Token, setup.ChanllegeTokenType, req.DID)
	if !ok {
		resp.Message = "Invalid token"
		return resp
	}
	dc := did.InitDIDLite(req.DID, c.didDir, nil)
	ok, err := dc.SignVerify([]byte(req.Token), req.Signature)
	if err != nil {
		if strings.Contains(err.Error(), "NLSS DID detected") || strings.Contains(err.Error(), "incompatible key format") {
			c.log.Error("NLSS DID detected during authentication. NLSS DIDs are DEPRECATED. Please use BIP DID", "did", req.DID, "error", err)
			resp.Message = "NLSS DID detected. Please use BIP DID"
		} else {
			c.log.Error("Failed to verify DID signature", "err", err)
			resp.Message = "Failed to verify DID signature"
		}
		return resp
	}
	if !ok {
		resp.Message = "Invalid signature"
		return resp
	}
	expiresAt := time.Now().Add(time.Minute * 10)
	tkn := c.generateDIDToken(setup.AccessTokenType, req.DID, true, expiresAt)
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
	privKey, err := ioutil.ReadFile(util.SanitizeDirPath(c.didDir) + didStr + "/" + constants.PvtKeyFileName)
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

// InitDIDModule initialises the DID sub-system without the full
// SetupCore() side-effects (listener, pubsub, peer manager).
// Must be called after RunIPFS() so that c.ipfs is non-nil.
func (c *Core) InitDIDModule() {
	if c.d == nil {
		// Ensure didDir has a trailing slash -- the did package uses raw string
		// concatenation (d.dir + DID) and requires a path separator at the end.
		c.didDir = util.SanitizeDirPath(c.didDir)
		c.d = did.InitDID(c.didDir, c.log, c.ipfs)
	}
}

func (c *Core) CreateDID(didCreate *types.DIDCreate, localDID bool) (did string, err error) {
	if localDID { // create key pair from mnemonic an d then did from the private key
		did, err = c.d.CreateDID(didCreate)
		if err != nil {
			return "", fmt.Errorf("Core: CreateDID: faile create did locally, err: %w", err)
		}
	} else { // create did from public key
		did, err = c.d.CreateDIDFromPubKey(didCreate)
		if err != nil {
			return "", fmt.Errorf("Core: CreateDID: faile create did from input public key, err: %w", err)
		}
	}

	if c.IsDIDExist(did) {
		return did, nil
	}

	dt := &models.DID{
		DID:    did,
		PeerID: c.peerID,
		Local:  localDID,
	}
	algoID, err := c.w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
	if err != nil {
		c.log.Error("Core: CreateDID: Failed to resolve did algo id", "algo", constants.DidAlgo_SECP256K1, "err", err)
		return "", err
	}
	dt.AlgoID = algoID

	err = c.w.CreateOrUpdateDID(dt)
	if err != nil {
		c.log.Error("Failed to create did in the wallet", "err", err)
		return "", err
	}

	return did, nil
}

func (c *Core) GetDIDs() []string {
	dt, err := c.w.GetAllDID(c.peerID)
	if err != nil {
		return nil
	}
	return dt
}

func (c *Core) IsDIDExist(did string) bool {
	_, err := c.w.GetDID(did)
	return err == nil
}

func (c *Core) AddDID(dc *types.DIDCreate) *model.BasicResponse {
	br := &model.BasicResponse{
		Status: false,
	}
	ds, err := c.d.CreateDID(dc)
	if err != nil {
		br.Message = err.Error()
		return br
	}
	dt := wallet.DID{
		DID: ds,
		// DIDDir: dc.Dir,
		// Config: dc.Config,
	}
	err = c.w.CreateDID(&dt)
	if err != nil {
		c.log.Error("Failed to create did in the wallet", "err", err)
		br.Message = err.Error()
		return br
	}

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
		c.log.Error("RegisterDID: Failed to get did channels")
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
	h := util.CalculateHash([]byte(c.peerID+did+t), constants.HashAlgorithm_SHA3_256)
	sig, err := dc.Sign(h)
	if err != nil {
		return fmt.Errorf("register did, failed to do signature, err: %w", err)
	}

	pm := &PeerMap{
		PeerID:    c.peerID,
		DID:       did,
		Signature: util.BytesToBase64(sig),
		Time:      t,
	}
	algoID, err := c.w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
	if err != nil {
		c.log.Error("registerDID: failed to resolve did algo id", "algo", constants.DidAlgo_SECP256K1, "err", err)
		return fmt.Errorf("registerDID: failed to resolve did algo id, err: %w", err)
	}
	pm.DIDAlgo = algoID
	err = c.publishPeerMap(pm)
	if err != nil {
		c.log.Error("Register DID, failed to publish peer did map", "err", err)
		return err
	}
	return nil
}

// This function, GetPeerDIDInfo, retrieves information about a peer's DID (Decentralized Identifier)
// from various sources, including the local database, an explorer, or directly from the peer.
// It returns a models.DID containing the peer's DID, Peer ID, and DID type,
// or an error if the information cannot be found.
func (c *Core) GetPeerDIDInfo(didStr string) (*models.DID, error) {
	c.log.Debug("Resolving peer info", "did", didStr)

	var peerID string

	// In case of xell wallet, TRIE testnet and Rubix testnet have same swarm key but different peerIDs.
	// So, an user should find another user's Rubix testnet DID-info in DIDTable and TRIE testnet DID-info in PeerDIDTable.
	if c.testnet {
		// 1. try DID table first
		if _, err := c.w.GetDID(didStr); err == nil {
			return &models.DID{
				DID:    didStr,
				PeerID: c.peerID,
			}, nil
		}

		// 2. If missing, try peer table
		peerID, _ = c.w.GetPeerID(didStr)

		if peerID != "" {
			return &models.DID{
				DID:    didStr,
				PeerID: peerID,
			}, nil
		}
	} else {
		// 1. Try peer table first
		peerID, _ = c.w.GetPeerID(didStr)

		if peerID != "" {
			return &models.DID{
				DID:    didStr,
				PeerID: peerID,
			}, nil
		}
	}

	// If peerID still missing, try resolving (via explorer or peer fetch)
	if peerID == "" {
		if !c.testnet {
			peerInfo, err := c.GetPeerFromExplorer(didStr)
			if err != nil {
				return nil, fmt.Errorf("explorer lookup failed: %w", err)
			}
			return peerInfo, nil
		}

		// Testnet: Cannot resolve peer without peerID
		// Calling getPeer(didStr) here creates a circular dependency:
		// GetPeerDIDInfo -> getPeer -> GetPeerDIDInfo -> ...
		// In testnet mode, peer information must be available locally or provided explicitly
		c.log.Error("PeerID not found in local storage", "did", didStr, "register the DID info")
		return nil, fmt.Errorf("peerID of  DID %s not found in local storage. Peer information not registered. register did to continue", didStr)
	}

	return &models.DID{
		DID:    didStr,
		PeerID: peerID,
	}, nil
}

func (c *Core) ArbitrarySign(reqID string, signReq *model.ArbitrarySignRequest) {
	signResp := c.arbitrarySign(reqID, signReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("arbitrary sign failed, Failed to get did channels")
		return
	}
	dc.OutChan <- signResp
}
func (c *Core) arbitrarySign(reqID string, signReq *model.ArbitrarySignRequest) *model.BasicResponse {
	signResp := &model.BasicResponse{
		Status: false,
	}

	// initiate the did with did crypto
	didCrypto, err := c.SetupDID(reqID, signReq.SignerDID)
	if err != nil {
		errMsg := fmt.Sprintf("arbitrary sign failed, failed to setup did, err : %v", err)
		c.log.Error(errMsg)
		signResp.Message = errMsg
		return signResp
	}

	// sign the given message with private key
	signatureBytes, err := didCrypto.Sign([]byte(signReq.MsgToSign))
	if err != nil {
		errMsg := fmt.Sprintf("arbitrary sign failed, err : %v", err)
		c.log.Error(errMsg)
		signResp.Message = errMsg
		return signResp
	}
	// convert signature bytes into string
	signature := util.BytesToBase64(signatureBytes)

	// verify the signature before returning
	verificationResult, err := didCrypto.SignVerify([]byte(signReq.MsgToSign), signatureBytes)
	if err != nil {
		errMsg := fmt.Sprintf("arbitrary sign failed, failed to verify signature, err : %v", err)
		c.log.Error(errMsg)
		signResp.Message = errMsg
		return signResp
	}

	if !verificationResult {
		c.log.Error("verification failed, signature is invalid")
		signResp.Message = "arbitrary sign failed, verification failed, signature is invalid"
	} else {
		signResp.Message = "arbitrary sign successful"
		signMap := model.Signature{
			Signature: signature,
		}
		signResp.Result = signMap
	}

	signResp.Status = true
	return signResp
}

func (c *Core) ArbitrarySignVerification(reqID string, verificationReq *model.SignVerificationRequest) (*model.BasicResponse, error) {
	verificationResp := &model.BasicResponse{
		Status: false,
	}

	// initiate the did with did crypto
	didCrypto, err := c.SetupForienDID(verificationReq.SignerDID, "")
	if err != nil {
		errMsg := fmt.Sprintf("failed to setup did for sign verification, err : %v", err)
		c.log.Error(errMsg)
		verificationResp.Message = errMsg
		return verificationResp, fmt.Errorf("%v", errMsg)
	}

	signatureBytes, err := util.Base64ToBytes(verificationReq.Signature)
	if err != nil {
		errMsg := fmt.Sprintf("ArbitrarySignVerification: failed to convert signature bytes to base64, err : %v", err)
		c.log.Error(errMsg)
		verificationResp.Message = errMsg
		return verificationResp, fmt.Errorf("%v", errMsg)
	}

	verificationResult, err := didCrypto.SignVerify([]byte(verificationReq.SignedMsg), signatureBytes)
	if err != nil {
		errMsg := fmt.Sprintf("failed to verify signature, err : %v", err)
		c.log.Error(errMsg)
		verificationResp.Message = errMsg
		return verificationResp, fmt.Errorf("%v", errMsg)
	}

	if verificationResult {
		verificationResp.Message = "verification passed, signature is valid"
	} else {
		c.log.Error("verification failed, signature is invalid")
		verificationResp.Message = "verification failed, signature is invalid"
	}

	verificationResp.Status = verificationResult
	return verificationResp, nil
}

func (c *Core) RemoveStaleDIDFromNetwork(reqID string, staleDID string) {
	br, err := c.removeStaleDIDFromNetwork(reqID, staleDID)
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}

	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("RemoveStaleDIDFromNetwork: Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

// remove stale DIDs from DIDTable and from peers' PeerDIDTable
func (c *Core) removeStaleDIDFromNetwork(reqID, staleDID string) (model.BasicResponse, error) {
	response := model.BasicResponse{
		Status: false,
	}

	// check if DID still holds tokens, prevent deletion if it does
	accInfo, err := c.GetRbtByDid(staleDID)
	if err != nil {
		c.log.Error("Failed to get account info for DID %v", staleDID)
		return response, err
	}
	if accInfo.RBTBalance == 0 &&
		accInfo.LockedRBT == 0 &&
		accInfo.PledgedRBT == 0 {

		// DID has no tokens, safe to delete
		c.log.Debug("*******did has no balance, safe to delete")
	} else {
		errMsg := fmt.Sprintf(
			"cannot remove DID: %v, holds RBT [%f free, %f locked, %f pledged]",
			staleDID,
			accInfo.RBTBalance,
			accInfo.LockedRBT,
			accInfo.PledgedRBT,
		)
		c.log.Error(errMsg)
		return response, fmt.Errorf(errMsg)
	}

	// ftInfo, err := c.GetFTInfoByDID(staleDID)
	// if err != nil {
	// 	c.log.Error("Failed to get ft info for DID %v", staleDID)
	// 	return response, err
	// }
	// if len(ftInfo) != 0 {
	// 	errMsg := fmt.Sprintf("cannot remove DID : %v, holds FTs : %v", staleDID, ftInfo)
	// 	c.log.Error(errMsg)
	// 	return response, fmt.Errorf(errMsg)
	// }

	// remove old-did from peers' DB :
	// 1. sign on the information to be published
	dc, err := c.SetupDID(reqID, staleDID)
	if err != nil {
		return response, fmt.Errorf("DID is not exist")
	}
	t := time.Now().String()
	h := util.CalculateHash([]byte(c.peerID+staleDID+t), "SHA3-256")
	sig, err := dc.Sign(h)
	if err != nil {
		return response, fmt.Errorf("remove stale did, failed to do signature")
	}

	// 2. publish the stale did and the signature
	pm := &PeerMap{
		PeerID:    c.peerID,
		DID:       staleDID,
		Signature: util.BytesToBase64(sig),
		Time:      t,
	}

	// TESTING
	signatureBytes, err := util.Base64ToBytes(pm.Signature)
	if err != nil {
		c.log.Error("peerCallback: failed to parse signature, err", err)
		return response, fmt.Errorf("remove stale did, failed sign test")
	}
	st, err := dc.SignVerify(h, signatureBytes)
	if err != nil || !st {
		if err != nil && (strings.Contains(err.Error(), "NLSS DID detected") || strings.Contains(err.Error(), "incompatible key format")) {
			c.log.Error("NLSS DID detected during stale peer removal. NLSS DIDs are DEPRECATED.", "did", pm.DID, "error", err)
		} else {
			c.log.Error("failed to remove stale peer, signature verification failed, err ", err)
		}
		return response, fmt.Errorf("remove stale did, failed sign test")
	}

	err = c.publishStalePeer(pm)
	if err != nil {
		c.log.Error("Remove DID from network, failed to publish peer did map", "err", err)
		return response, err
	}

	// remove old-did from DIDTable
	err = c.w.RemoveDID(staleDID)
	if err != nil {
		response.Message = err.Error()
		return response, err
	}

	// remove old-did folder
	os.RemoveAll(path.Join(c.didDir, staleDID))

	response.Status = true
	response.Message = "successfully erased staled did"
	c.log.Info(response.Message)
	return response, nil
}
