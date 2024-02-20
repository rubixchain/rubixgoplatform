package did

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDQuorum_Lt will handle light DID
type DIDQuorum_Lt struct {
	did     string
	dir     string
	pwd     string
	privKey crypto.PrivateKey
	pubKey  crypto.PublicKey
}

// InitDIDQuorum_Lt will return the Quorum did handle in light mode
func InitDIDQuorum_Lt(did string, baseDir string, pwd string) *DIDQuorum_Lt {
	d := &DIDQuorum_Lt{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
	if d.pwd != "" {
		privKey, err := ioutil.ReadFile(d.dir + PvtKeyFileName)
		if err != nil {
			return nil
		}
		d.privKey, _, err = crypto.DecodeKeyPair(d.pwd, privKey, nil)
		if err != nil {
			return nil
		}
	}

	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFileName)
	if err != nil {
		return nil
	}
	_, d.pubKey, err = crypto.DecodeKeyPair("", nil, pubKey)
	if err != nil {
		return nil
	}
	return d
}

func (d *DIDQuorum_Lt) GetDID() string {
	return d.did
}

func (d *DIDQuorum_Lt) GetSignVersion() int {
	return PkiVersion
}

// Sign will return the singature of the DID
func (d *DIDQuorum_Lt) Sign(hash string) ([]byte, []byte, error) {
	if d.privKey == nil {
		return nil, nil, fmt.Errorf("private key is not initialized")
	}
	pvtKeySign, err := d.PvtSign([]byte(hash))
	// byteImg, err := util.GetPNGImagePixels(d.dir + PvtShareFileName)

	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}

	bs := []byte{}
	return bs, pvtKeySign, err
}

// verify the quorum's nlss based signature
func (d *DIDQuorum_Lt) NlssVerify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
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
func (d *DIDQuorum_Lt) PvtSign(hash []byte) ([]byte, error) {
	if d.privKey == nil {
		return nil, fmt.Errorf("private key is not initialized")
	}
	ps, err := crypto.Sign(d.privKey, hash)
	if err != nil {
		return nil, err
	}
	return ps, nil
}
func (d *DIDQuorum_Lt) PvtVerify(hash []byte, sign []byte) (bool, error) {
	if !crypto.Verify(d.pubKey, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
