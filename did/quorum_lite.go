package did

import (
	"fmt"
	"io/ioutil"
	"path"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/crypto"
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
		privKey, err := ioutil.ReadFile(path.Join(d.dir, constants.PvtKeyFileName))
		if err != nil {
			fmt.Println("private key must be in wallet")
		} else {
			d.privKey, _, err = crypto.DecodeBIPKeyPair(d.pwd, privKey, nil)
			if err != nil {
				return nil
			}
		}
	}

	pubKey, err := ioutil.ReadFile(path.Join(d.dir, constants.PubKeyFileName))
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

func (d *DIDQuorumLite) Sign(hash []byte) ([]byte, error) {
	privKey, err := ioutil.ReadFile(path.Join(d.dir, constants.PvtKeyFileName))
	if err != nil {
		fmt.Println("requesting signature from BIP wallet")
		// _, walletSignature, err := d.signRequest(hash)
		// if err != nil {
		// 	fmt.Println("failed sign request, err:", err)
		// 	return nil, err
		// }
		// fmt.Println("received signature:", walletSignature)

		// isValidSig, err := d.PvtVerify(hash, walletSignature)
		// if err != nil || !isValidSig {
		// 	fmt.Println("invalid sign data:", util.HexToStr(hash), "err:", err)
		// }
		return nil, fmt.Errorf("triggered lite quorum")
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
func (d *DIDQuorumLite) SignVerify(hash []byte, sign []byte) (bool, error) {

	pubKeyPath := path.Join(d.dir, constants.PubKeyFileName)
	pubKey, err := ioutil.ReadFile(pubKeyPath)
	if err != nil {
		return false, err
	}

	_, pubKeyByte, err := crypto.DecodeBIPKeyPair("", nil, pubKey)
	if err != nil {
		return false, err
	}

	pubkeyback, err := secp256k1.ParsePubKey(pubKeyByte)
	if err != nil {
		return false, fmt.Errorf("NLSS DID detected at QUORUM role (incompatible key format). NLSS DIDs are DEPRECATED. Please use BIP DID for quorum. DID: %s, Error: %v", d.did, err)
	}
	if pubkeyback == nil {
		return false, fmt.Errorf("NLSS DID detected at QUORUM role (public key parsing returned nil). NLSS DIDs are DEPRECATED. Please use BIP DID for quorum. DID: %s", d.did)
	}

	pubKeySer := pubkeyback.ToECDSA()

	if !crypto.BIPVerify(pubKeySer, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}
