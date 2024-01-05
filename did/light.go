package did

import (
	// "bytes"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/rubixchain/rubixgoplatform/crypto"
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
	return d.pwd, nil
}

func (d *DIDLight) GetDID() string {
	return d.did
}

func (d *DIDLight) Sign(hash string) ([]byte, []byte, error) {
	pvtKeySign, err := d.PvtSign([]byte(hash))
	bs := []byte{}

	return bs, pvtKeySign, err
}
func (d *DIDLight) PvtSign(hash []byte) ([]byte, error) {
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

func (d *DIDLight) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	return d.PvtVerify([]byte(hash), pvtKeySIg)
}
func (d *DIDLight) PvtVerify(hash []byte, sign []byte) (bool, error) {
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
