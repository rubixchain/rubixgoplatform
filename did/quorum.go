package did

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDQuorum will handle basic DID
type DIDQuorum struct {
	did     string
	dir     string
	pwd     string
	privKey crypto.PrivateKey
	pubKey  crypto.PublicKey
}

// InitDIDBasic will return the basic did handle
func InitDIDQuorumc(did string, baseDir string, pwd string) *DIDQuorum {
	d := &DIDQuorum{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
	if d.pwd != "" {
		privKey, err := ioutil.ReadFile(d.dir + QuorumPvtKeyFileName)
		if err != nil {
			return nil
		}
		d.privKey, _, err = crypto.DecodeKeyPair(d.pwd, privKey, nil)
		if err != nil {
			return nil
		}
	}

	pubKey, err := ioutil.ReadFile(d.dir + QuorumPubKeyFileName)
	if err != nil {
		return nil
	}
	_, d.pubKey, err = crypto.DecodeKeyPair("", nil, pubKey)
	if err != nil {
		return nil
	}
	return d
}

func (d *DIDQuorum) GetDID() string {
	return d.did
}

// Sign will return the singature of the DID
func (d *DIDQuorum) Sign(hash string) ([]byte, []byte, error) {
	if d.privKey == nil {
		return nil, nil, fmt.Errorf("private key is not initialized")
	}
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
	pvtKeySign, err := crypto.Sign(d.privKey, []byte(hashPvtSign))
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
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pSig), "SHA3-256"))
	if !crypto.Verify(d.pubKey, []byte(hashPvtSign), pvtKeySIg) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
func (d *DIDQuorum) PvtSign(hash []byte) ([]byte, error) {
	if d.privKey == nil {
		return nil, fmt.Errorf("private key is not initialized")
	}
	ps, err := crypto.Sign(d.privKey, hash)
	if err != nil {
		return nil, err
	}
	return ps, nil
}
func (d *DIDQuorum) PvtVerify(hash []byte, sign []byte) (bool, error) {
	if !crypto.Verify(d.pubKey, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
