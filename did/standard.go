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

// DIDStandard will handle basic DID
type DIDStandard struct {
	did string
	dir string
	ch  *DIDChan
}

// InitDIDStandard will return the basic did handle
func InitDIDStandard(did string, baseDir string, ch *DIDChan) *DIDStandard {
	return &DIDStandard{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", ch: ch}
}

func (d *DIDStandard) getSignature(hash []byte) ([]byte, error) {
	if d.ch == nil || d.ch.InChan == nil || d.ch.OutChan == nil {
		return nil, fmt.Errorf("Invalid configuration")
	}
	sr := &SignResponse{
		Status:  true,
		Message: "Signature needed",
		Result: SignReqData{
			ID:   d.ch.ID,
			Mode: StandardDIDMode,
			Hash: hash,
		},
	}
	d.ch.OutChan <- sr
	var ch interface{}
	select {
	case ch = <-d.ch.InChan:
	case <-time.After(d.ch.Timeout):
		return nil, fmt.Errorf("Timeout, failed to get signature")
	}

	srd, ok := ch.(SignRespData)
	if !ok {
		return nil, fmt.Errorf("Invalid data received on the channel")
	}
	return srd.Signature.Signature, nil
}

func (d *DIDStandard) GetDID() string {
	return d.did
}

// Sign will return the singature of the DID
func (d *DIDStandard) Sign(hash string) ([]byte, []byte, error) {
	byteImg, err := util.GetPNGImagePixels(d.dir + PvtShareFileName)

	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}

	ps := util.ByteArraytoIntArray(byteImg)

	randPosObject := util.RandomPositions("signer", hash, 32, ps)

	finalPos := randPosObject.PosForSign
	pvtPos := util.GetPrivatePositions(finalPos, ps)
	pvtPosStr := util.IntArraytoStr(pvtPos)
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pvtPosStr), "SHA3-256"))
	pvtKeySign, err := d.getSignature([]byte(hashPvtSign))
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
func (d *DIDStandard) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
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

func (d *DIDStandard) PvtSign(hash []byte) ([]byte, error) {
	pvtKeySign, err := d.getSignature(hash)
	if err != nil {
		return nil, err
	}
	return pvtKeySign, nil
}
func (d *DIDStandard) PvtVerify(hash []byte, sign []byte) (bool, error) {
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
