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

// DIDChild will handle basic DID
type DIDChild struct {
	did     string
	baseDir string
	dir     string
	ch      *DIDChan
	pwd     string
}

// InitDIDChild will return the basic did handle
func InitDIDChild(did string, baseDir string, ch *DIDChan) *DIDChild {
	return &DIDChild{did: did, baseDir: util.SanitizeDirPath(baseDir), dir: util.SanitizeDirPath(baseDir) + did + "/", ch: ch}
}

func InitDIDChildWithPassword(did string, baseDir string, pwd string) *DIDChild {
	return &DIDChild{did: did, baseDir: util.SanitizeDirPath(baseDir), dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
}

func (d *DIDChild) getPassword() (string, error) {
	if d.pwd != "" {
		return d.pwd, nil
	}
	if d.ch == nil || d.ch.InChan == nil || d.ch.OutChan == nil {
		return "", fmt.Errorf("Invalid configuration")
	}
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
	d.pwd = srd.Password
	return d.pwd, nil
}

func (d *DIDChild) GetDID() string {
	return d.did
}

// Sign will return the singature of the DID
func (d *DIDChild) Sign(hash string) ([]byte, []byte, error) {

	rb, err := ioutil.ReadFile(d.dir + MasterDIDFileName)
	if err != nil {
		return nil, nil, err
	}
	mdid := string(rb)
	byteImg, err := util.GetPNGImagePixels(d.baseDir + mdid + "/" + PvtShareFileName)

	if err != nil {
		return nil, nil, err
	}

	ps := util.ByteArraytoIntArray(byteImg)

	randPosObject := util.RandomPositions("signer", hash, 32, ps)

	finalPos := randPosObject.PosForSign
	pvtPos := util.GetPrivatePositions(finalPos, ps)
	pvtPosStr := util.IntArraytoStr(pvtPos)

	//create a signature using the private key
	//1. read and extrqct the private key
	privKey, err := ioutil.ReadFile(d.dir + PvtKeyFileName)
	if err != nil {
		return nil, nil, err
	}
	pwd, err := d.getPassword()
	if err != nil {
		return nil, nil, err
	}
	PrivateKey, _, err := crypto.DecodeKeyPair(pwd, privKey, nil)
	if err != nil {
		return nil, nil, err
	}
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pvtPosStr), "SHA3-256"))
	pvtKeySign, err := crypto.Sign(PrivateKey, []byte(hashPvtSign))
	if err != nil {
		return nil, nil, err
	}
	bs, err := util.BitstreamToBytes(pvtPosStr)
	if err != nil {
		return nil, nil, err
	}
	return bs, pvtKeySign, err
}

// Sign will verifyt he signature
func (d *DIDChild) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	rb, err := ioutil.ReadFile(d.dir + MasterDIDFileName)
	if err != nil {
		return false, err
	}
	mdid := string(rb)

	didImg, err := util.GetPNGImagePixels(d.baseDir + mdid + "/" + DIDImgFileName)
	if err != nil {
		return false, err
	}
	pubImg, err := util.GetPNGImagePixels(d.baseDir + mdid + "/" + PubShareFileName)

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

func (d *DIDChild) PvtSign(hash []byte) ([]byte, error) {
	privKey, err := ioutil.ReadFile(d.dir + PvtKeyFileName)
	if err != nil {
		return nil, err
	}
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
	return pvtKeySign, nil
}
func (d *DIDChild) PvtVerify(hash []byte, sign []byte) (bool, error) {
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
