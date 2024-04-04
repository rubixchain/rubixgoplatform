package did

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/util"
)

// DIDDummy will handle basic DID
type DIDDummy struct {
	did    string
	pvtKey []byte
	pubKey []byte
}

// InitDIDBasic will return the basic did handle
func InitDIDDummy(did string) *DIDDummy {
	pvtKey, pubKey, err := crypto.GenerateKeyPair(&crypto.CryptoConfig{Alg: crypto.ECDSAP256})
	if err != nil {
		return nil
	}
	return &DIDDummy{did: did, pvtKey: pvtKey, pubKey: pubKey}
}

// Sign will return the singature of the DID
func (d *DIDDummy) Sign(hash string) ([]byte, []byte, error) {

	PrivateKey, _, err := crypto.DecodeKeyPair("", d.pvtKey, nil)
	if err != nil {
		return nil, nil, err
	}
	rb := util.GetRandBytes(32)
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(hash+d.did+util.HexToStr(rb)), "SHA3-256"))
	pvtKeySign, err := crypto.Sign(PrivateKey, []byte(hashPvtSign))
	if err != nil {
		return nil, nil, err
	}
	return rb, pvtKeySign, err
}

// Sign will verifyt he signature
func (d *DIDDummy) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	// read senderDID

	_, pubKeyByte, err := crypto.DecodeKeyPair("", nil, d.pubKey)
	if err != nil {
		return false, err
	}
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(hash+d.did+util.HexToStr(pvtShareSig)), "SHA3-256"))
	if !crypto.Verify(pubKeyByte, []byte(hashPvtSign), pvtKeySIg) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}

func (d *DIDDummy) PvtSign(hash []byte) ([]byte, error) {

	PrivateKey, _, err := crypto.DecodeKeyPair("", d.pvtKey, nil)
	if err != nil {
		return nil, err
	}
	pvtKeySign, err := crypto.Sign(PrivateKey, hash)
	if err != nil {
		return nil, err
	}
	return pvtKeySign, nil
}
func (d *DIDDummy) PvtVerify(hash []byte, sign []byte) (bool, error) {

	_, pubKeyByte, err := crypto.DecodeKeyPair("", nil, d.pubKey)
	if err != nil {
		return false, err
	}
	if !crypto.Verify(pubKeyByte, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
