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

// DIDWallet will handle basic DID
type DIDWallet struct {
	did string
	dir string
	ch  *DIDChan
}

// InitDIDWallet will return the basic did handle
func InitDIDWallet(did string, baseDir string, ch *DIDChan) *DIDWallet {
	return &DIDWallet{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", ch: ch}
}

func (d *DIDWallet) getSignature(hash []byte, onlyPrivKey bool) ([]byte, []byte, error) {
	if d.ch == nil || d.ch.InChan == nil || d.ch.OutChan == nil {
		return nil, nil, fmt.Errorf("Invalid configuration")
	}
	sr := &SignResponse{
		Status:  true,
		Message: "Signature needed",
		Result: SignReqData{
			ID:          d.ch.ID,
			Mode:        WalletDIDMode,
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
	return srd.Signature.Pixels, srd.Signature.Signature, nil
}

func (d *DIDWallet) GetDID() string {
	return d.did
}

// Sign will return the singature of the DID
func (d *DIDWallet) Sign(hash string) ([]byte, []byte, error) {
	bs, pvtKeySign, err := d.getSignature([]byte(hash), false)
	if err != nil {
		return nil, nil, err
	}
	return bs, pvtKeySign, err
}

// Sign will verifyt he signature
func (d *DIDWallet) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
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

func (d *DIDWallet) PvtSign(hash []byte) ([]byte, error) {
	_, pvtKeySign, err := d.getSignature(hash, true)
	if err != nil {
		return nil, err
	}
	return pvtKeySign, nil
}
func (d *DIDWallet) PvtVerify(hash []byte, sign []byte) (bool, error) {
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
