package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

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

func (c *Core) CreateDID(didCreate *types.DIDCreate) (did string, err error) {
	if didCreate.PubKey == "" { // create key pair from mnemonic and then did from the private key
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

	// Always record local ownership, even if the DID row already exists. A prior
	// remote rubix_did announcement may have inserted it as local=false; creating
	// it here means this node holds the mnemonic, so upsert it as local=true.
	// CreateOrUpdateDID's ON CONFLICT DO UPDATE promotes the existing row.
	dt := &models.DID{
		DID:    did,
		PeerID: c.peerID,
		Local:  true,
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

func (c *Core) AddDID(dc *types.DIDCreate) *models.BasicResponse {
	br := &models.BasicResponse{
		Status: false,
	}
	ds, err := c.d.CreateDID(dc)
	if err != nil {
		br.Message = err.Error()
		return br
	}
	dt := models.DID{
		DID: ds,
		// DIDDir: dc.Dir,
		// Config: dc.Config,
	}
	err = c.w.CreateOrUpdateDID(&dt)
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
	br := models.BasicResponse{
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
	c.log.Debug("GetPeerDIDInfo: Resolving peer info", "did", didStr)

	peerID, err := c.w.GetPeerID(didStr)
	if err != nil || peerID == "" {
		// Local dids table miss. The rubix_did announcement is ephemeral, so a
		// node that was offline when this DID registered never learned its
		// peerID. Ask the network (fullnodes) on demand before giving up.
		if resolved, ok := c.resolvePeerInfoViaPubsub(didStr); ok {
			peerID = resolved
		} else {
			if err != nil {
				c.log.Error("GetPeerDIDInfo: Failed to get peer ID", "did", didStr, "error", err)
			}
			return nil, fmt.Errorf("peer ID not found for DID: %s", didStr)
		}
	}

	return &models.DID{
		DID:    didStr,
		PeerID: peerID,
	}, nil
}

func (c *Core) ArbitrarySign(reqID string, signReq *models.ArbitrarySignRequest) {
	signResp := c.arbitrarySign(reqID, signReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("arbitrary sign failed, Failed to get did channels")
		return
	}
	dc.OutChan <- signResp
}
func (c *Core) arbitrarySign(reqID string, signReq *models.ArbitrarySignRequest) *models.BasicResponse {
	signResp := &models.BasicResponse{
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
		signMap := models.ArbitrarySignature{
			Signature: signature,
		}
		signResp.Result = signMap
	}

	signResp.Status = true
	return signResp
}

func (c *Core) ArbitrarySignVerification(reqID string, verificationReq *models.SignVerificationRequest) (*models.BasicResponse, error) {
	verificationResp := &models.BasicResponse{
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
