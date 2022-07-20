package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/util"
)

type randPosObj struct {
	originalPos []int `json:"originalPos"`
	posForSign  []int `json:"posForSign"`
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

		pos = make([]int, 8)
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

		if strings.Compare(role, "signer") == 1 {

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

	result := randPosObj{originalPos, posForSign}

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

func getSignFromShares(filePath string, hash string) string {

	byteImg, err := util.GetPNGImagePixels(filePath)

	if err != nil {
		fmt.Println(err)
		return "Could not read File " + err.Error()
	}

	privateIntegerArray1 := byteArraytoIntArray(byteImg)

	var randPosObject randPosObj
	P := randomPositions("signer", hash, 32, privateIntegerArray1)

	json.Unmarshal(P, &randPosObject)

	var finalPos []int = randPosObject.posForSign
	var p1Sign []int = getPrivatePositions(finalPos, privateIntegerArray1)

	return intArraytoStr(p1Sign)
}

func main() {
	byteImg, err := util.GetPNGImagePixels("/Applications/Rubix/DATA/QmU2hWEpeRhTCE9V7FDQvGj4twfN25A4ofZJU6mXLo1NDq/PrivateShare.png")

	if err != nil {
		fmt.Println(err)
	} else {

		intArray := byteArraytoIntArray(byteImg)

		//util.FileWrite("/Users/rubix_1/Documents/RubixGO/rubixgoplatform/core/test/a.txt", byteImg)

		writeStringToFile(intArraytoStr(intArray))

	}
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
	f, err := os.Create("/Users/rubix_1/Documents/RubixGO/rubixgoplatform/core/test/a.txt")

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
