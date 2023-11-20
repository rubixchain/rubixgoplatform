package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/util"
)

type RandPos struct {
	OriginalPos []int `json:"originalPos"`
	PosForSign  []int `json:"posForSign"`
}

type SignVerifyObj struct {
	Did       string `json:"did"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}

type DIDBasic struct {
	did  string
	path string
	pwd  string
}

func main() {
	//hash := HexToStr(util.CalculateHash([]byte("Boom"), "SHA3-256"))

	fmt.Println(util.CalculateHash([]byte("Boom"), "SHA3-256"))
	/* signature, err := basics.Sign("/Users/rubix_1/Downloads/PrivateShare.png", (hash))

	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("\n hash of boom: ", hash)

	fmt.Println("\n signature using private share : ", signature) */
	/* signverifyData := SignVerifyObj{
		Did: "bafybmig3qzwpjksxeyxk4vck7l7qs3f42rmwhw7ow3lmxwlvfnxesufova", Hash: (hash), Signature: signature}

	signverifyDataObj, err := json.Marshal(signverifyData)

	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("\nverifying signature : ", VerifySignature(signverifyDataObj)) */
}

func RandomPositions(role string, hash string, numOfPositions int, pvt1 []int) []byte {
	var u, l, m int = 0, 0, 0

	hashCharacters := make([]int, 256)
	randomPositions := make([]int, 32)
	randPos := make([]int, 256)
	var finalPositions, pos []int
	originalPos := make([]int, 32)
	posForSign := make([]int, 32*8)

	for k := 0; k < numOfPositions; k++ {

		temp, err := strconv.ParseInt(string(hash[k]), 16, 32)
		if err != nil {
			fmt.Println(err)
		}
		hashCharacters[k] = int(temp)
		randomPositions[k] = (((2402 + hashCharacters[k]) * 2709) + ((k + 2709) + hashCharacters[(k)])) % 2048
		originalPos[k] = (randomPositions[k] / 8) * 8

		pos = make([]int, 32)
		pos[k] = originalPos[k]
		randPos[k] = pos[k]

		finalPositions = make([]int, 8)

		for p := 0; p < 8; p++ {

			posForSign[u] = randPos[k]
			randPos[k]++
			u++

			finalPositions[l] = pos[k]
			pos[k]++
			l++

			if l == 8 {
				l = 0
			}
		}
		if strings.Compare(role, "signer") == 0 {
			var p1 []int = GetPrivatePositions(finalPositions, pvt1)
			fmt.Println("originalpos :::: ", IntArraytoStr(originalPos))
			hash = HexToStr(util.CalculateHash([]byte(hash+IntArraytoStr(originalPos)+IntArraytoStr(p1)), "SHA3-256"))

		} else {
			p1 := make([]int, 8)

			for i := 0; i < 8; i++ {
				p1[i] = pvt1[m]
				m++
			}
			hash = HexToStr(util.CalculateHash([]byte(hash+IntArraytoStr(originalPos)+IntArraytoStr(p1)), "SHA3-256"))

		}
	}
	result := RandPos{
		OriginalPos: originalPos, PosForSign: posForSign}

	result_obj, err := json.Marshal(result)

	if err != nil {
		fmt.Println(err)
	}

	return result_obj
}

func GetPrivatePositions(positions []int, privateArray []int) []int {

	privatePositions := make([]int, len(positions))

	for k := 0; k < len(positions); k++ {
		var a int = positions[k]
		var b int = privateArray[a]

		privatePositions[k] = b
	}

	return privatePositions
}

func IntArraytoStr(intArray []int) string {
	var result bytes.Buffer
	for i := 0; i < len(intArray); i++ {
		if intArray[i] == 1 {
			result.WriteString("1")
		} else {
			result.WriteString("0")
		}
	}
	return result.String()
}

func StringToIntArray(data string) []int {

	reuslt := make([]int, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '1' {
			reuslt[i] = 1
		} else {
			reuslt[i] = 0
		}
	}
	return reuslt
}

func (d *DIDBasic) GetSignFromShares(filePath string, hash string) (string, error) {

	byteImg, err := util.GetPNGImagePixels(filePath)

	if err != nil {
		fmt.Println(err)
		return "Could not read File ", err
	}

	privateIntegerArray1 := ByteArraytoIntArray(byteImg)

	var randPosObject RandPos
	P := RandomPositions("signer", hash, 32, privateIntegerArray1)

	json.Unmarshal([]byte(P), &randPosObject)

	var finalPos []int = randPosObject.PosForSign
	var p1Sign []int = GetPrivatePositions(finalPos, privateIntegerArray1)

	//create a signature using the private key
	//1. read and extrqct the private key
	privKey, err := ioutil.ReadFile(d.path + "/" + d.did + "/pvtKey.pem")
	if err != nil {
		return "Could not read PvtKey.pem file", err
	}
	pubKey, err := ioutil.ReadFile(d.path + "/" + d.did + "/pubKey.pem")
	crypto.DecodeKeyPair(d.pwd, privKey, pubKey)

	return IntArraytoStr(p1Sign), err
}

func ByteArraytoIntArray(byteArray []byte) []int {

	result := make([]int, len(byteArray)*8)
	for i, b := range byteArray {
		for j := 0; j < 8; j++ {
			result[i*8+j] = int(b >> uint(7-j) & 0x01)
		}
	}
	return result
}

func ByteArraytostr(byteArray []byte) string {
	return bytes.NewBuffer(byteArray).String()
}

func WriteStringToFile(data string) {
	f, err := os.Create("/Users/rubix_1/Documents/RubixGO/rubixgoplatform/core/test/c.txt")

	if err != nil {
		fmt.Println(err)
	}

	defer f.Close()

	_, err2 := f.WriteString(data)

	if err2 != nil {
		fmt.Println(err)
	}

	fmt.Println("done")
}

func VerifySignature(detailsString []byte) bool {

	var result bool
	// make ipfs connection -> to do
	var signVerifyObject SignVerifyObj

	json.Unmarshal([]byte(detailsString), &signVerifyObject)

	decentralizedID := signVerifyObject.Did
	hash := signVerifyObject.Hash
	signature := signVerifyObject.Signature

	fmt.Println("\n ", decentralizedID, hash, signature)

	//synd data table -> to do

	//get walletahs from datatable based on did and call node data

	// read senderDID
	didByteImg, didByteImgerr := util.GetPNGImagePixels("/Users/rubix_1/Downloads/DID.png")
	wIdByteImg, wIdByteImgerr := util.GetPNGImagePixels("/Users/rubix_1/Downloads/PublicShare.png")

	if didByteImgerr != nil {
		fmt.Println(didByteImgerr)
	} else if wIdByteImgerr != nil {
		fmt.Println(wIdByteImgerr)
	}

	senderDIDBin := IntArraytoStr(ByteArraytoIntArray(didByteImg))

	walletID := IntArraytoStr(ByteArraytoIntArray(wIdByteImg))

	var senderWalletID strings.Builder

	senderSign := StringToIntArray(signature)

	var randomPositionsObject RandPos

	P := RandomPositions("verifier", hash, 32, senderSign)

	json.Unmarshal([]byte(P), &randomPositionsObject)

	posForSign := randomPositionsObject.PosForSign
	originalPos := randomPositionsObject.OriginalPos

	for _, positionsLevelTwoTrail := range posForSign {
		senderWalletID.WriteString(string(walletID[positionsLevelTwoTrail]))
	}

	recombinedResult := GetPos(senderWalletID.String(), signature)

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

	return result
}

func GetPos(s1, s2 string) string {
	var i, j, temp, temp1, sum int

	if len(s1) != len(s2) || len(s1) < 1 {
		return ""
	}
	var tempo strings.Builder

	for i = 0; i < len(s1); i += 8 {
		sum = 0
		for j = i; j < i+8; j++ {
			temp = int(s1[j]) - 0
			temp1 = int(s2[j]) - 0

			sum += temp * temp1
		}
		sum %= 2
		tempo.WriteString(strconv.Itoa(sum))
	}

	return tempo.String()
}

func HexToStr(ByteArray []byte) string {
	dst := make([]byte, hex.EncodedLen(len(ByteArray)))
	hex.Encode(dst, ByteArray)

	return string(dst)
}
