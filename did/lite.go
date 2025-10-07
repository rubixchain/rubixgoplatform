package did

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDLite will handle Light DID
type DIDLite struct {
	did string
	dir string
	ch  *DIDChan
	pwd string
}

// InitDIDLite will return the Light did handle
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
func (d *DIDLite) Sign(hash string) ([]byte, []byte, error) {
	pvtKeySign, err := d.PvtSign([]byte(hash))
	bs := []byte{}

	return bs, pvtKeySign, err
}

// verify nlss based signatures
func (d *DIDLite) NlssVerify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	//read senderDID
	didImg, err := util.GetPNGImagePixels(d.dir + DIDImgFileName)
	if err != nil {
		return false, err
	}
	pubImg, err := util.GetPNGImagePixels(d.dir + PubShareFileName)

	if err != nil {
		return false, err
	}

	pSig := util.BytesToBitstream(pvtShareSig)

	ps := util.StringToIntArray(pSig)

	didBin := util.ByteArraytoIntArray(didImg)
	pubBin := util.ByteArraytoIntArray(pubImg)
	pubPos := util.RandomPositions("verifier", hash, 32, ps)
	pubPosInt := util.GetPrivatePositions(pubPos.PosForSign, pubBin)
	pubStr := util.IntArraytoStr(pubPosInt)
	orgPos := make([]int, len(pubPos.OriginalPos))
	for i := range pubPos.OriginalPos {
		orgPos[i] = pubPos.OriginalPos[i] / 8
	}
	didPosInt := util.GetPrivatePositions(orgPos, didBin)
	didStr := util.IntArraytoStr(didPosInt)
	cb := nlss.Combine2Shares(nlss.ConvertBitString(pSig), nlss.ConvertBitString(pubStr))

	db := nlss.ConvertBitString(didStr)

	if !bytes.Equal(cb, db) {
		return false, fmt.Errorf("failed to verify")
	}

	//create a signature using the private key
	//1. read and extrqct the private key
	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFileName)
	if err != nil {
		return false, err
	}

	_, pubKeyByte, err := crypto.DecodeKeyPair("", nil, pubKey)
	if err != nil {
		return false, err
	}

	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pSig), "SHA3-256"))
	if !crypto.Verify(pubKeyByte, []byte(hashPvtSign), pvtKeySIg) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil

}

func (d *DIDLite) PvtSign(hash []byte) ([]byte, error) {
	privKey, err := os.ReadFile(d.dir + PvtKeyFileName)
	if err != nil {
		fmt.Println("requesting signature from BIP wallet")
		walletSignature, err := d.getSignature(hash)
		if err != nil {
			fmt.Println("failed sign request, err:", err)
			return nil, err
		}
		fmt.Println("received signature:", walletSignature)

		isValidSig, err := d.PvtVerify(hash, walletSignature)
		if err != nil || !isValidSig {
			fmt.Println("invalid sign data:", util.HexToStr(hash), "err:", err)
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

	pubkeyback, _ := secp256k1.ParsePubKey(pubKeyByte)
	pubKeySer := pubkeyback.ToECDSA()
	if !crypto.BIPVerify(pubKeySer, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
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
