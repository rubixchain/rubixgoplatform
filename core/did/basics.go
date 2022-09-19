package did

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/EnsurityTechnologies/enscrypt"
	"github.com/rubixchain/rubixgoplatform/core/util"
)

// DIDBasic will handle basic DID
type DIDBasic struct {
	did  string
	path string
	pwd  string
}

// InitDIDBasic will return the basic did handle
func InitDIDBasic(did string, path string, pwd string) *DIDBasic {
	return &DIDBasic{did: did, path: path, pwd: pwd}
}

// Sign will return the singature of the DID
func (d *DIDBasic) Sign(path, hash string) (string, string, error) {
	// pf := util.SanitizeDirPath(d.path) + d.did + "/" + PvtShareImgFile
	// f, err := os.Open(pf)
	// if err != nil {
	// 	return nil, err
	// }
	// defer f.Close()
	// img, _, err := image.Decode(f)
	// img.
	byteImg, err := util.GetPNGImagePixels(path + "/" + d.did + "/DID.png")

	if err != nil {
		fmt.Println(err)
		return "Could not read File ", "", err
	}

	privateIntegerArray1 := util.ByteArraytoIntArray(byteImg)

	var randPosObject util.RandPosObj
	P, err := util.RandomPositions("signer", hash, 32, privateIntegerArray1)

	if err != nil {
		return "JSON Error", "", err
	}

	json.Unmarshal([]byte(P), &randPosObject)

	var finalPos []int = randPosObject.PosForSign
	var p1Sign []int = util.GetPrivatePositions(finalPos, privateIntegerArray1)

	//create a signature using the private key
	//1. read and extrqct the private key
	privKey, err := ioutil.ReadFile(d.path + "/" + d.did + "/pvtKey.pem")
	if err != nil {
		return "Could not read PvtKey.pem file", "", err
	}
	pubKey, err := ioutil.ReadFile(d.path + "/" + d.did + "/pubKey.pem")
	if err != nil {
		return "Could not read Pubkey.pem file", "", err
	}
	PrivateKey, PublicKey, err := enscrypt.DecodeKeyPair(d.pwd, privKey, pubKey)

	if err != nil {
		fmt.Printf("PublicKey: %v\n", PublicKey)
		return "keys cannot be decrypted", "", err
	}

	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(util.IntArraytoStr(p1Sign)), "SHA3-256"))
	pvtKeySign, err := enscrypt.Sign(PrivateKey, []byte(hashPvtSign))

	return string(pvtKeySign), hashPvtSign, err

}

// Sign will verifyt he signature
func (d *DIDBasic) Verify(coord []int, didSig string, hash string) (bool, error) {

	var result bool
	// make ipfs connection -> to do
	//var signVerifyObject SignVerifyObj

	//json.Unmarshal([]byte(detailsString), &signVerifyObject)

	decentralizedID := d.did
	signature := didSig
	path := d.path

	//fmt.Println("\n ", decentralizedID, hash, signature)

	//synd data table -> to do

	//get walletahs from datatable based on did and call node data

	// read senderDID
	didByteImg, didByteImgerr := util.GetPNGImagePixels(path + "/" + decentralizedID + "/DID.png")
	wIdByteImg, wIdByteImgerr := util.GetPNGImagePixels(path + "/" + decentralizedID + "/PublicShare.png")

	if didByteImgerr != nil {
		//fmt.Println(didByteImgerr)
		return false, didByteImgerr
	} else if wIdByteImgerr != nil {
		//fmt.Println(wIdByteImgerr)
		return false, wIdByteImgerr
	}

	senderDIDBin := util.IntArraytoStr(util.ByteArraytoIntArray(didByteImg))

	walletID := util.IntArraytoStr(util.ByteArraytoIntArray(wIdByteImg))

	var senderWalletID strings.Builder

	senderSign := util.StringToIntArray(signature)

	var randomPositionsObject util.RandPosObj

	P, err := util.RandomPositions("verifier", hash, 32, senderSign)

	if err != nil {
		return false, err
	}

	json.Unmarshal([]byte(P), &randomPositionsObject)

	posForSign := randomPositionsObject.PosForSign
	originalPos := randomPositionsObject.OriginalPos

	for _, positionsLevelTwoTrail := range posForSign {
		senderWalletID.WriteString(string(walletID[positionsLevelTwoTrail]))
	}

	recombinedResult := util.GetPos(senderWalletID.String(), signature)

	positionsLevelZero := make([]int, 32)

	for k := 0; k < 32; k++ {
		positionsLevelZero[k] = (originalPos[k] / 8)
	}

	var decentralizedIDForAuth strings.Builder
	for _, value := range positionsLevelZero {
		decentralizedIDForAuth.WriteString(string(senderDIDBin[value]))
	}

	fmt.Println("recombined : ", recombinedResult)
	fmt.Println("decentralizedIDForAuth : ", decentralizedIDForAuth.String())

	if strings.Compare(recombinedResult, decentralizedIDForAuth.String()) == 0 {
		result = true
	} else {
		result = false
	}

	return result, err

	//return false, nil
}
