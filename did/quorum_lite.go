package did

import (
	"fmt"
	"io/ioutil"

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
		privKey, err := ioutil.ReadFile(d.dir + constants.PvtKeyFileName)
		if err != nil {
			fmt.Println("private key must be in wallet")
		} else {
			d.privKey, _, err = crypto.DecodeBIPKeyPair(d.pwd, privKey, nil)
			if err != nil {
				return nil
			}
		}
	}

	pubKey, err := ioutil.ReadFile(d.dir + constants.PubKeyFileName)
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
	privKey, err := ioutil.ReadFile(d.dir + constants.PvtKeyFileName)
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

	pubKeyPath := d.dir + constants.PubKeyFileName
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

// func (d *DIDQuorumLite) signRequest(hash []byte) ([]byte, []byte, error) {
// 	if d.ch == nil || d.ch.InChan == nil || d.ch.OutChan == nil {
// 		return nil, nil, fmt.Errorf("Invalid configuration")
// 	}
// 	sr := &SignResponse{
// 		Status:  true,
// 		Message: "Signature needed",
// 		Result: SignReqData{
// 			ID:          d.ch.ID,
// 			Mode:        WalletDIDMode,
// 			Hash:        hash,
// 			OnlyPrivKey: true,
// 		},
// 	}
// 	d.ch.OutChan <- sr
// 	var ch interface{}
// 	select {
// 	case ch = <-d.ch.InChan:
// 	case <-time.After(d.ch.Timeout):
// 		return nil, nil, fmt.Errorf("Timeout, failed to get signature")
// 	}

// 	srd, ok := ch.(SignRespData)
// 	if !ok {
// 		return nil, nil, fmt.Errorf("Invalid data received on the channel")
// 	}
// 	return srd.Signature.Pixels, srd.Signature.Signature, nil
// }

// func (d *DIDQuorumLite) signRequest(hash []byte) ([]byte, error) {
// 	data := map[string]interface{}{
// 		"data": util.HexToStr(hash),
// 		"did":  d.did,
// 	}
// 	bodyJSON, err := json.Marshal(data)
// 	if err != nil {
// 		fmt.Println("Error marshaling JSON:", err)
// 		return nil, err
// 	}
// 	return bodyJSON, nil
// 	// port := string(20009)
// 	// url := "http://localhost:8080/sign"
// 	// req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
// 	// if err != nil {
// 	// 	fmt.Println("Error creating HTTP request:", err)
// 	// 	return nil, err
// 	// }

// 	// req.Header.Set("Content-Type", "application/json; charset=UTF-8")

// 	// client := &http.Client{}
// 	// resp, err := client.Do(req)
// 	// if err != nil {
// 	// 	fmt.Println("Error sending HTTP request:", err)
// 	// 	resp.Body.Close()
// 	// 	return nil, err
// 	// }
// 	// defer resp.Body.Close()
// 	// fmt.Println("Response Status:", resp.Status)
// 	// data2, err := io.ReadAll(resp.Body)
// 	// if err != nil {
// 	// 	fmt.Printf("Error reading response body: %s\n", err)
// 	// 	return nil, err
// 	// }
// 	// // Process the data as needed
// 	// fmt.Println("Response Body in did request :", string(data2))

// 	// var response map[string]interface{}
// 	// err = json.Unmarshal(data2, &response)
// 	// if err != nil {
// 	// 	fmt.Println("Error unmarshaling response:", err)
// 	// }

// 	// signaturestr := response["signature"].(string)
// 	// signature, err := hex.DecodeString(signaturestr)
// 	// if err != nil {
// 	// 	fmt.Printf("failed to decode signature string, err: %v", err)
// 	// 	return nil, err
// 	// }
// 	// return signature, nil
// }
