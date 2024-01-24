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

// DIDLight will handle Light DID
type DIDLight struct {
	did string
	dir string
	ch  *DIDChan
	pwd string
}

// InitDIDLight will return the Light did handle
func InitDIDLight(did string, baseDir string, ch *DIDChan) *DIDLight {
	return &DIDLight{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", ch: ch}
}

func InitDIDLightWithPassword(did string, baseDir string, pwd string) *DIDLight {
	return &DIDLight{did: did, dir: util.SanitizeDirPath(baseDir) + did + "/", pwd: pwd}
}

func (d *DIDLight) getPassword() (string, error) {
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
			Mode: LightDIDMode,
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
	fmt.Println("getting pwd in light mode")
	return d.pwd, nil
}

func (d *DIDLight) GetDID() string {
	return d.did
}

func (d *DIDLight) GetSignVersion() int {
	fmt.Println("PkiVersion")
	return PkiVersion
}

func (d *DIDLight) Sign(hash string) ([]byte, []byte, error) {
	pvtKeySign, err := d.PvtSign([]byte(hash))
	bs := []byte{}

	fmt.Println("pki sign in light mode")
	fmt.Println("signing data:", hash)
	return bs, pvtKeySign, err
}

func (d *DIDLight) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	fmt.Println("verifying nlss sign from light mode")
	// if signVersion == PkiVersion {
	// 	return d.PvtVerify([]byte(hash), pvtKeySIg)
	// } else {
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
	// }

}

func (d *DIDLight) PvtSign(hash []byte) ([]byte, error) {
	privKey, err := ioutil.ReadFile(d.dir + PvtKeyFileName)
	if err != nil {
		return nil, err
	}
	fmt.Println("pvt sign in light mode")
	pwd, err := d.getPassword()
	if err != nil {
		return nil, err
	}
	fmt.Println("got pwd in light mode")
	PrivateKey, _, err := crypto.DecodeKeyPair(pwd, privKey, nil)
	if err != nil {
		return nil, err
	}
	fmt.Println("pvt key decoded in light mode")
	pvtKeySign, err := crypto.Sign(PrivateKey, hash)
	if err != nil {
		return nil, err
	}
	return pvtKeySign, nil
}

func (d *DIDLight) PvtVerify(hash []byte, sign []byte) (bool, error) {
	fmt.Println("verifying pvt sign from light mode")
	fmt.Println("verifying data:", hash)
	fmt.Println("verifying sign:", sign)
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
