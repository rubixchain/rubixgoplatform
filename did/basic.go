package did

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDBasic will handle basic DID
type DIDBasic struct {
	did string
	dir string
	ch  *DIDChan
	pwd string
}

// InitDIDBasic will return the basic did handle
func InitDIDBasic(did string, baseDir string, ch *DIDChan) *DIDBasic {
	return &DIDBasic{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", ch: ch}
}

func InitDIDBasicWithPassword(did string, baseDir string, pwd string) *DIDBasic {
	return &DIDBasic{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
}

func (d *DIDBasic) getPassword() (string, error) {
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
			Mode: BasicDIDMode,
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

func (d *DIDBasic) GetDID() string {
	return d.did
}

// When the did creation and signing is done in Basic mode,
// this function returns the sign version as NLSSVersion = 1
func (d *DIDBasic) GetSignType() int {
	return NlssVersion
}

// getSignature requests signature from external source (wallet/mobile app) via channel
func (d *DIDBasic) getSignature(hash []byte, onlyPrivKey bool) ([]byte, []byte, error) {
	if d.ch == nil || d.ch.InChan == nil || d.ch.OutChan == nil {
		return nil, nil, fmt.Errorf("Invalid configuration, channel not available for external signing")
	}
	fmt.Printf("BasicDID.getSignature: Requesting external signature via channel\n")
	sr := &SignResponse{
		Status:  true,
		Message: "Signature needed",
		Result: SignReqData{
			ID:          d.ch.ID,
			Mode:        BasicDIDMode,
			Hash:        hash,
			OnlyPrivKey: onlyPrivKey,
		},
	}
	d.ch.OutChan <- sr
	var ch interface{}
	select {
	case ch = <-d.ch.InChan:
	case <-time.After(d.ch.Timeout):
		return nil, nil, fmt.Errorf("Timeout, failed to get signature")
	}

	srd, ok := ch.(SignRespData)
	if !ok {
		return nil, nil, fmt.Errorf("Invalid data received on the channel")
	}
	fmt.Printf("BasicDID signature received from external source: Pixels=%d bytes, ECDSA=%d bytes\n", len(srd.Signature.Pixels), len(srd.Signature.Signature))
	return srd.Signature.Pixels, srd.Signature.Signature, nil
}

// Sign will return the singature of the DID
func (d *DIDBasic) Sign(hash string) ([]byte, []byte, error) {
	fmt.Printf("BasicDID.Sign called: hash=%s (len=%d)\n", hash, len(hash))

	// Check if pvtShare.png exists
	pvtSharePath := d.dir + PvtShareFileName
	if _, err := ioutil.ReadFile(pvtSharePath); err != nil {
		// File doesn't exist or can't be read
		fmt.Printf("BasicDID.Sign: pvtShare.png not found or inaccessible, attempting external signing\n")

		// Try external signing via channel (like WalletDID)
		if d.ch != nil && d.ch.InChan != nil && d.ch.OutChan != nil {
			fmt.Printf("BasicDID.Sign: Using external signing (channel-based)\n")
			bs, pvtKeySign, err := d.getSignature([]byte(hash), false)
			if err != nil {
				return nil, nil, fmt.Errorf("External signing failed: %v", err)
			}
			fmt.Printf("BasicDID Sign (external): NLSS=%d bytes, ECDSA=%d bytes\n", len(bs), len(pvtKeySign))
			return bs, pvtKeySign, nil
		}

		// Neither local file nor channel available
		return nil, nil, fmt.Errorf("Cannot sign: pvtShare.png not found and no external signing channel available")
	}

	// File exists, use local signing
	fmt.Printf("BasicDID.Sign: Using local signing (file-based)\n")
	byteImg, err := util.GetPNGImagePixels(pvtSharePath)
	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}

	ps := util.ByteArraytoIntArray(byteImg)

	randPosObject := util.RandomPositions("signer", hash, 32, ps)

	finalPos := randPosObject.PosForSign
	pvtPos := util.GetPrivatePositions(finalPos, ps)
	pvtPosStr := util.IntArraytoStr(pvtPos)

	//create a signature using the private key
	//1. read and extrqct the private key
	// privKey, err := ioutil.ReadFile(d.dir + PvtKeyFileName)
	// if err != nil {
	// 	return nil, nil, err
	// }
	// pwd, err := d.getPassword()
	// if err != nil {
	// 	return nil, nil, err
	// }
	// PrivateKey, _, err := crypto.DecodeKeyPair(pwd, privKey, nil)
	// if err != nil {
	// 	return nil, nil, err
	// }
	// hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pvtPosStr), "SHA3-256"))
	// pvtKeySign, err := crypto.Sign(PrivateKey, []byte(hashPvtSign))
	// if err != nil {
	// 	return nil, nil, err
	// }
	var pvtKeySign []byte
	bs, err := util.BitstreamToBytes(pvtPosStr)
	if err != nil {
		return nil, nil, err
	}
	fmt.Printf("BasicDID Sign (local): NLSS=%d bytes, ECDSA=%d bytes\n", len(bs), len(pvtKeySign))
	return bs, pvtKeySign, err
}

// Sign will verifyt he signature
func (d *DIDBasic) NlssVerify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	fmt.Printf("BasicDID.NlssVerify called: hash=%s (len=%d), pvtShareSigLen=%d\n", hash, len(hash), len(pvtShareSig))
	// read senderDID
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

	fmt.Printf("NlssVerify: pSigLen=%d, pubStrLen=%d, didStrLen=%d, cbLen=%d, dbLen=%d\n",
		len(pSig), len(pubStr), len(didStr), len(cb), len(db))
	fmt.Printf("NlssVerify: cb=%x\n", cb)
	fmt.Printf("NlssVerify: db=%x\n", db)
	fmt.Printf("NlssVerify: bytes.Equal(cb, db)=%v\n", bytes.Equal(cb, db))

	if !bytes.Equal(cb, db) {
		return false, fmt.Errorf("failed to verify")
	}

	//create a signature using the private key
	//1. read and extrqct the private key
	// pubKey, err := ioutil.ReadFile(d.dir + PubKeyFileName)
	// if err != nil {
	// 	return false, err
	// }
	// _, pubKeyByte, err := crypto.DecodeKeyPair("", nil, pubKey)
	// if err != nil {
	// 	return false, err
	// }
	// hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pSig), "SHA3-256"))
	// if !crypto.Verify(pubKeyByte, []byte(hashPvtSign), pvtKeySIg) {
	// 	return false, fmt.Errorf("failed to verify nlss private key singature")
	// }
	return true, nil
}

func (d *DIDBasic) PvtSign(hash []byte) ([]byte, error) {
	fmt.Printf("BasicDID.PvtSign called: hashLen=%d\n", len(hash))

	// Check if pvtKey.pem exists
	pvtKeyPath := d.dir + PvtKeyFileName
	privKey, err := ioutil.ReadFile(pvtKeyPath)
	if err != nil {
		// File doesn't exist or can't be read
		fmt.Printf("BasicDID.PvtSign: pvtKey.pem not found or inaccessible, attempting external signing\n")

		// Try external signing via channel (like WalletDID)
		if d.ch != nil && d.ch.InChan != nil && d.ch.OutChan != nil {
			fmt.Printf("BasicDID.PvtSign: Using external signing (channel-based)\n")
			_, pvtKeySign, err := d.getSignature(hash, true) // onlyPrivKey = true
			if err != nil {
				return nil, fmt.Errorf("External PKI signing failed: %v", err)
			}
			fmt.Printf("BasicDID PvtSign (external): ECDSA=%d bytes\n", len(pvtKeySign))
			return pvtKeySign, nil
		}

		// Neither local file nor channel available
		return nil, fmt.Errorf("Cannot sign: pvtKey.pem not found and no external signing channel available")
	}

	// File exists, use local signing
	fmt.Printf("BasicDID.PvtSign: Using local signing (file-based)\n")
	pwd, err := d.getPassword()
	if err != nil {
		return nil, err
	}
	PrivateKey, _, err := crypto.DecodeKeyPair(pwd, privKey, nil)
	if err != nil {
		return nil, err
	}
	pvtKeySign, err := crypto.Sign(PrivateKey, hash)
	if err != nil {
		return nil, err
	}
	fmt.Printf("BasicDID PvtSign (local): ECDSA=%d bytes\n", len(pvtKeySign))
	return pvtKeySign, nil
}
func (d *DIDBasic) PvtVerify(hash []byte, sign []byte) (bool, error) {
	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFileName)
	if err != nil {
		return false, err
	}
	_, pubKeyByte, err := crypto.DecodeKeyPair("", nil, pubKey)
	if err != nil {
		return false, err
	}
	if !crypto.Verify(pubKeyByte, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
