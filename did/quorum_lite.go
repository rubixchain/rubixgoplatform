package did

import (
	"bytes"
	"fmt"
	"io/ioutil"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDQuorumLite will handle lite DID
type DIDQuorumLite struct {
	did     string
	dir     string
	pwd     string
	privKey crypto.PrivateKey
	pubKey  crypto.PublicKey
}

// InitDIDQuorumLite will return the Quorum did handle in lite mode
func InitDIDQuorumLite(did string, baseDir string, pwd string) *DIDQuorumLite {
	d := &DIDQuorumLite{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
	if d.pwd != "" {
		privKey, err := ioutil.ReadFile(d.dir + PvtKeyFileName)
		if err != nil {
			return nil
		}
		d.privKey, _, err = crypto.DecodeBIPKeyPair(d.pwd, privKey, nil)
		if err != nil {
			return nil
		}
	}

	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFileName)
	if err != nil {
		return nil
	}
	_, d.pubKey, err = crypto.DecodeBIPKeyPair("", nil, pubKey)
	if err != nil {
		return nil
	}
	return d
}

func (d *DIDQuorumLite) GetDID() string {
	return d.did
}

func (d *DIDQuorumLite) GetSignType() int {
	return BIPVersion
}

// Sign will return the singature of the DID
func (d *DIDQuorumLite) Sign(hash string) ([]byte, []byte, error) {
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
func (d *DIDQuorumLite) NlssVerify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
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

	pubKey, err := ioutil.ReadFile(d.dir + PubKeyFileName)
	if err != nil {
		return false, err
	}

	_, pubKeyByte, err := crypto.DecodeKeyPair("", nil, pubKey)
	if err != nil {
		return false, err
	}

	if !crypto.Verify(pubKeyByte, []byte(hashPvtSign), pvtKeySIg) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
func (d *DIDQuorumLite) PvtSign(hash []byte) ([]byte, error) {
	privKey, err := ioutil.ReadFile(d.dir + PvtKeyFileName)
	if err != nil {
		return nil, err
	}

	Privatekey, _, err := crypto.DecodeBIPKeyPair(d.pwd, privKey, nil)
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
func (d *DIDQuorumLite) PvtVerify(hash []byte, sign []byte) (bool, error) {
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
