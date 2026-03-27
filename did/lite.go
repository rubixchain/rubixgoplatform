package did

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDLite will handle Light DID
type DIDLite struct {
	did string
	dir string
	ch  *DIDChan
	pwd string
}

// InitDIDLite will return the Lite did handle
func InitDIDLite(did string, baseDir string, ch *DIDChan) *DIDLite {
	return &DIDLite{did: did, dir: path.Join(util.SanitizeDirPath(baseDir), did), ch: ch}
}

func InitDIDLiteWithPassword(did string, baseDir string, pwd string) *DIDLite {
	return &DIDLite{did: did, dir: path.Join(util.SanitizeDirPath(baseDir), did), pwd: pwd}
}

func (d *DIDLite) getPassword() (string, error) {
	// Check if password is already cached in this DID object
	if d.pwd != "" {
		return d.pwd, nil
	}

	if d.ch == nil || d.ch.InChan == nil || d.ch.OutChan == nil {
		return "", fmt.Errorf("Invalid configuration")
	}

	// Check request-scoped cache first (read lock)
	d.ch.PasswordMutex.RLock()
	if d.ch.PasswordSet && d.ch.CachedPassword != "" {
		cachedPwd := d.ch.CachedPassword
		d.ch.PasswordMutex.RUnlock()

		// Cache in DID object for faster access
		d.pwd = cachedPwd
		return d.pwd, nil
	}
	d.ch.PasswordMutex.RUnlock()

	// Acquire write lock to request password
	d.ch.PasswordMutex.Lock()
	defer d.ch.PasswordMutex.Unlock()

	// Double-check: another goroutine might have set password while waiting
	if d.ch.PasswordSet && d.ch.CachedPassword != "" {
		d.pwd = d.ch.CachedPassword
		return d.pwd, nil
	}

	// Request password from user
	sr := &types.SignResponse{
		Status:  true,
		Message: "Password needed",
		Result: types.SignReqData{
			ID: d.ch.ID,
		},
	}
	d.ch.OutChan <- sr

	var ch interface{}
	select {
	case ch = <-d.ch.InChan:
	case <-time.After(d.ch.Timeout):
		return "", fmt.Errorf("Timeout, failed to get password")
	}

	srd, ok := ch.(types.SignRespData)
	if !ok {
		return "", fmt.Errorf("Invalid data received on the channel")
	}

	// Cache password for this request
	d.ch.CachedPassword = srd.Password
	d.ch.PasswordSet = true
	d.pwd = srd.Password // Also cache in DID object

	return d.pwd, nil
}

func (d *DIDLite) GetDID() string {
	return d.did
}

func (d *DIDLite) Sign(hash []byte) ([]byte, error) {
	privKey, err := os.ReadFile(path.Join(d.dir, constants.PvtKeyFileName))
	if err != nil {
		walletSignature, err := d.getSignature(hash)
		if err != nil {
			return nil, err
		}

		isValidSig, err := d.SignVerify(hash, walletSignature)
		if err != nil || !isValidSig {
			return nil, err
		}
		return walletSignature, nil
	}

	pwd, err := d.getPassword()
	if err != nil {
		return nil, err
	}

	Privatekey, _, err := crypto.DecodeBIPKeyPair(pwd, privKey, nil)
	if err != nil {
		return nil, err
	}

	privkeyback := secp256k1.PrivKeyFromBytes(Privatekey)
	privKeySer := privkeyback.ToECDSA()
	pvtKeySign, err := crypto.BIPSign(privKeySer, hash)
	if err != nil {
		return nil, err
	}
	return pvtKeySign, nil
}

// Verify PKI based signature
func (d *DIDLite) SignVerify(hash []byte, sign []byte) (bool, error) {
	pubKey, err := ioutil.ReadFile(path.Join(d.dir, constants.PubKeyFileName))
	if err != nil {
		return false, err
	}

	_, pubKeyByte, err := crypto.DecodeBIPKeyPair("", nil, pubKey)
	if err != nil {
		return false, err
	}

	pubkeyback, err := secp256k1.ParsePubKey(pubKeyByte)
	if err != nil {
		return false, fmt.Errorf("NLSS DID detected (incompatible key format). NLSS DIDs are DEPRECATED. Please use BIP DID instead. DID: %s, Error: %w", d.did, err)
	}
	if pubkeyback == nil {
		return false, fmt.Errorf("NLSS DID detected (public key parsing returned nil). NLSS DIDs are DEPRECATED. Please use BIP DID instead. DID: %s", d.did)
	}

	pubKeySer := pubkeyback.ToECDSA()
	if !crypto.BIPVerify(pubKeySer, hash, sign) {
		return false, fmt.Errorf("failed to verify private key signature")
	}
	return true, nil
}

func (d *DIDLite) getSignature(hash []byte) ([]byte, error) {
	if d.ch == nil || d.ch.InChan == nil || d.ch.OutChan == nil {
		return nil, fmt.Errorf("invalid configuration")
	}
	sr := &types.SignResponse{
		Status:  true,
		Message: "Signature needed",
		Result: types.SignReqData{
			ID:   d.ch.ID,
			Hash: hash,
		},
	}
	d.ch.OutChan <- sr
	var ch interface{}
	select {
	case ch = <-d.ch.InChan:
	case <-time.After(d.ch.Timeout):
		return nil, fmt.Errorf("timeout, failed to get signature")
	}

	srd, ok := ch.(types.SignRespData)
	if !ok {
		return nil, fmt.Errorf("invalid data received on the channel")
	}
	// convert base64 signature into byte array
	return util.Base64ToBytes(srd.Signature)
}
