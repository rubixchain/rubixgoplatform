package did

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/EnsurityTechnologies/enscrypt"
	"github.com/rubixchain/rubixgoplatform/core/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDQuorum will handle basic DID
type DIDQuorum struct {
	did string
	dir string
	pwd string
}

// InitDIDBasic will return the basic did handle
func InitDIDQuorumc(did string, baseDir string, pwd string) *DIDQuorum {
	return &DIDQuorum{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
}

// Sign will return the singature of the DID
func (d *DIDQuorum) Sign(hash string) ([]byte, []byte, error) {
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

	//create a signature using the private key
	//1. read and extrqct the private key
	privKey, err := ioutil.ReadFile(d.dir + QuorumPvtKeyFileName)
	if err != nil {
		return nil, nil, err
	}

	PrivateKey, _, err := enscrypt.DecodeKeyPair(d.pwd, privKey, nil)
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
func (d *DIDQuorum) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
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
	pubKey, err := ioutil.ReadFile(d.dir + QuorumPubKeyFileName)
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

func (d *DIDQuorum) PvtSign(hash []byte) ([]byte, error) {
	return nil, nil
}
func (d *DIDQuorum) PvtVerify(hash []byte, sign []byte) (bool, error) {
	return false, nil
}
