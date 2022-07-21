package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/util"
)

type RandPosObj struct {
	OriginalPos []int `json:"originalPos"`
	PosForSign  []int `json:"posForSign"`
}

type SignVerifyObj struct {
	Did       string `json:"did"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}

func main() {
	/* byteImg, err := util.GetPNGImagePixels("/Applications/Rubix/DATA/QmU2hWEpeRhTCE9V7FDQvGj4twfN25A4ofZJU6mXLo1NDq/PrivateShare.png")

	if err != nil {
		fmt.Println(err)
	} else {

		intArray := byteArraytoIntArray(byteImg)

		//util.FileWrite("/Users/rubix_1/Documents/RubixGO/rubixgoplatform/core/test/a.txt", byteImg)

		writeStringToFile(intArraytoStr(intArray))

	} */

	hash := util.CalculateHash([]byte("testingGOPvtShareSignature"), "SHA3-256")

	fmt.Println("hash calulated", hash)

	pubimag, err := util.GetPNGImagePixels("/Applications/Rubix/DATA/QmU2hWEpeRhTCE9V7FDQvGj4twfN25A4ofZJU6mXLo1NDq/DID.png")
	if err != nil {
		fmt.Println(err)
	}
	writeStringToFile(intArraytoStr(byteArraytoIntArray(pubimag)))

	/* signature := getSignFromShares("/Applications/Rubix/DATA/QmU2hWEpeRhTCE9V7FDQvGj4twfN25A4ofZJU6mXLo1NDq/PrivateShare.png", string(hash))

	fmt.Println("\n signature using private share : ", signature)
	signverifyData := SignVerifyObj{
		Did: "QmU2hWEpeRhTCE9V7FDQvGj4twfN25A4ofZJU6mXLo1NDq", Hash: string(hash), Signature: signature}

	signverifyDataObj, err := json.Marshal(signverifyData)

	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("\nverifying signature : ", verifySignature(signverifyDataObj)) */
}

func randomPositions(role string, hash string, numOfPositions int, pvt1 []int) []byte {
	var u, l, m int = 0, 0, 0

	hashCharacters := make([]int, 256)
	randomPositions := make([]int, 32)
	randPos := make([]int, 256)
	var finalPositions, pos []int
	originalPos := make([]int, 32)
	posForSign := make([]int, 32*8)

	for k := 0; k < numOfPositions; k++ {
		hashCharacters[k] = int(hash[k])
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
			//fmt.Println(role)
			var p1 []int = getPrivatePositions(finalPositions, pvt1)

			hash = string(util.CalculateHash([]byte(hash+intArraytoStr(finalPositions)+intArraytoStr(p1)), "SHA3-256"))

		} else {
			p1 := make([]int, 8)

			for i := 0; i < 8; i++ {
				p1[i] = pvt1[m]
				m++
			}
			hash = string(util.CalculateHash([]byte(hash+intArraytoStr(finalPositions)+intArraytoStr(p1)), "SHA3-256"))

		}
	}
	result := RandPosObj{
		OriginalPos: originalPos, PosForSign: posForSign}

	result_obj, err := json.Marshal(result)

	if err != nil {
		fmt.Println(err)
	}

	return result_obj
}

func getPrivatePositions(positions []int, privateArray []int) []int {

	//var length int = len(positions)
	privatePositions := make([]int, len(positions))

	for k := 0; k < len(positions); k++ {
		var a int = positions[k]
		var b int = privateArray[a]

		privatePositions[k] = b
	}
	return privatePositions
}

func intArraytoStr(intArray []int) string {
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

func stringToIntArray(data string) []int {

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

func getSignFromShares(filePath string, hash string) string {

	byteImg, err := util.GetPNGImagePixels(filePath)

	if err != nil {
		fmt.Println(err)
		return "Could not read File " + err.Error()
	}

	privateIntegerArray1 := byteArraytoIntArray(byteImg)

	var randPosObject RandPosObj
	P := randomPositions("signer", hash, 32, privateIntegerArray1)

	json.Unmarshal([]byte(P), &randPosObject)

	var finalPos []int = randPosObject.PosForSign
	var p1Sign []int = getPrivatePositions(finalPos, privateIntegerArray1)

	return intArraytoStr(p1Sign)
}

func byteArraytoIntArray(byteArray []byte) []int {

	result := make([]int, len(byteArray)*8)
	for i, b := range byteArray {
		for j := 0; j < 8; j++ {
			result[i*8+j] = int(b >> uint(7-j) & 0x01)
		}
	}
	return result
}

func writeStringToFile(data string) {
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

func verifySignature(detailsString []byte) bool {

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
	didByteImg, didByteImgerr := util.GetPNGImagePixels("/Applications/Rubix/DATA/" + decentralizedID + "/DID.png")
	wIdByteImg, wIdByteImgerr := util.GetPNGImagePixels("/Applications/Rubix/DATA/" + decentralizedID + "/PublicShare.png")

	if didByteImgerr != nil {
		fmt.Println(didByteImgerr)
	} else if wIdByteImgerr != nil {
		fmt.Println(wIdByteImgerr)
	}

	senderDIDBin := intArraytoStr(byteArraytoIntArray(didByteImg))

	walletID := intArraytoStr(byteArraytoIntArray(wIdByteImg))

	var senderWalletID strings.Builder

	senderSign := stringToIntArray(signature)

	var randomPositionsObject RandPosObj

	P := randomPositions("verifier", hash, 32, senderSign)

	json.Unmarshal([]byte(P), &randomPositionsObject)

	posForSign := randomPositionsObject.PosForSign
	originalPos := randomPositionsObject.OriginalPos

	for i := range posForSign {
		senderWalletID.WriteString(string(walletID[i]))
	}

	recombinedResult := getPos(senderWalletID.String(), signature)

	positionsLevelZero := make([]int, 32)

	for k := 0; k < 32; k++ {
		positionsLevelZero[k] = (originalPos[k] / 8)
	}

	var decentralizedIDForAuth strings.Builder
	for i := range positionsLevelZero {
		decentralizedIDForAuth.WriteString(string(senderDIDBin[i]))
	}

	if strings.Compare(recombinedResult, decentralizedIDForAuth.String()) == 0 {
		result = true
	} else {
		result = false
	}

	return result
}

func getPos(s1, s2 string) string {
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
		tempo.WriteString(string(sum))
	}

	return tempo.String()
}
