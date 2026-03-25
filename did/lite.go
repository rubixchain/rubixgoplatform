package did

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/rubixchain/rubixgoplatform/crypto"
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
	return &DIDLite{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", ch: ch}
}

func InitDIDLiteWithPassword(did string, baseDir string, pwd string) *DIDLite {
	return &DIDLite{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
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
	sr := &SignResponse{
		Status:  true,
		Message: "Password needed",
		Result: SignReqData{
			ID:   d.ch.ID,
			Mode: LiteDIDMode,
		},
	}
	d.ch.OutChan <- sr

	var ch interface{}
	select {
	case ch = <-d.ch.InChan:
	case <-time.After(d.ch.Timeout):
		return "", fmt.Errorf("Timeout, failed to get password")
	}

	srd, ok := ch.(SignRespData)
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

// When the did creation and signing is done in Light mode,
// this function returns the sign version as BIPVersion = 0
func (d *DIDLite) GetSignType() int {
	return BIPVersion
}

// PKI based sign in lite mode
// In lite mode, the sign function returns only the private signature, unlike the basic mode
func (d *DIDLite) Sign(hash string) ([]byte, []byte, error) { //TODO : should return one signature only
	pvtKeySign, err := d.PvtSign([]byte(hash))
	bs := []byte{}

	return bs, pvtKeySign, err
}

func (d *DIDLite) PvtSign(hash []byte) ([]byte, error) {
	privKey, err := os.ReadFile(d.dir + PvtKeyFileName)
	if err != nil {
		walletSignature, err := d.getSignature(hash)
		if err != nil {
			return nil, err
		}

		isValidSig, err := d.PvtVerify(hash, walletSignature)
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
func (d *DIDLite) PvtVerify(hash []byte, sign []byte) (bool, error) {
	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFileName)
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
	sr := &SignResponse{
		Status:  true,
		Message: "Signature needed",
		Result: SignReqData{
			ID:          d.ch.ID,
			Mode:        LiteDIDMode,
			Hash:        hash,
			OnlyPrivKey: true,
		},
	}
	d.ch.OutChan <- sr
	var ch interface{}
	select {
	case ch = <-d.ch.InChan:
	case <-time.After(d.ch.Timeout):
		return nil, fmt.Errorf("timeout, failed to get signature")
	}

	srd, ok := ch.(SignRespData)
	if !ok {
		return nil, fmt.Errorf("invalid data received on the channel")
	}
	return srd.Signature.Signature, nil
}
