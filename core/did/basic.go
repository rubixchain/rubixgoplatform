package did

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/EnsurityTechnologies/enscrypt"
	"github.com/rubixchain/rubixgoplatform/core/nlss"
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
	return &DIDBasic{did: did, dir: baseDir, pwd: pwd}
}

func (d *DIDBasic) getPassword() (string, error) {
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

// Sign will return the singature of the DID
func (d *DIDBasic) Sign(hash string) ([]byte, []byte, error) {
	byteImg, err := util.GetPNGImagePixels(d.dir + PvtShareImgFile)

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
	privKey, err := ioutil.ReadFile(d.dir + PvtKeyFile)
	if err != nil {
		return nil, nil, err
	}
	pwd, err := d.getPassword()
	if err != nil {
		return nil, nil, err
	}
	PrivateKey, _, err := enscrypt.DecodeKeyPair(pwd, privKey, nil)
	if err != nil {
		return nil, nil, err
	}
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pvtPosStr), "SHA3-256"))
	pvtKeySign, err := enscrypt.Sign(PrivateKey, []byte(hashPvtSign))
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
func (d *DIDBasic) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	// read senderDID
	didImg, err := util.GetPNGImagePixels(d.dir + DIDImgFile)
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
	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFile)
	if err != nil {
		return false, err
	}
	_, pubKeyByte, err := enscrypt.DecodeKeyPair("", nil, pubKey)
	if err != nil {
		return false, err
	}
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pSig), "SHA3-256"))
	if !enscrypt.Verify(pubKeyByte, []byte(hashPvtSign), pvtKeySIg) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}

func (d *DIDBasic) PvtSign(hash []byte) ([]byte, error) {
	privKey, err := ioutil.ReadFile(d.dir + PvtKeyFile)
	if err != nil {
		return nil, err
	}
	pwd, err := d.getPassword()
	if err != nil {
		return nil, err
	}
	PrivateKey, _, err := enscrypt.DecodeKeyPair(pwd, privKey, nil)
	if err != nil {
		return nil, err
	}
	pvtKeySign, err := enscrypt.Sign(PrivateKey, hash)
	if err != nil {
		return nil, err
	}
	return pvtKeySign, nil
}
func (d *DIDBasic) PvtVerify(hash []byte, sign []byte) (bool, error) {
	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFile)
	if err != nil {
		return false, err
	}
	_, pubKeyByte, err := enscrypt.DecodeKeyPair("", nil, pubKey)
	if err != nil {
		return false, err
	}
	if !enscrypt.Verify(pubKeyByte, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
