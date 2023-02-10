package token

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/EnsurityTechnologies/logger"
	ipfsnode "github.com/ipfs/go-ipfs-api"
)

type TokenAuth struct {
	log  logger.Logger
	ipfs *ipfsnode.Shell
}

func (ta *TokenAuth) getWholeTokenValue(wholeTokenName string) (int, string) {

	response, err := ta.ipfs.Cat(wholeTokenName)
	if err != nil {
		ta.log.Error("Error occured while fetching token content", "error", err)
		return -1, ""
	}

	result, err := ioutil.ReadAll(response)

	if err != nil {
		ta.log.Error("Error in reading data", "error", err)
		return -1, ""
	}

	trimmedResult := strings.TrimSpace(string(result))

	response.Close()

	tokenLevel := string(trimmedResult[:len(trimmedResult)-64])
	tokenCountHash := string(trimmedResult[len(trimmedResult)-64:])

	//fmt.Println(len(tokenCountHash), "token len")

	tokenLevelInt, err := strconv.Atoi(tokenLevel)
	if err != nil {
		fmt.Println("Error occured while retriving token level", "error", err)
	}
	//	fmt.Println("tokenLevelInt is " + strconv.Itoa(tokenLevelInt))
	//	fmt.Println("tokenCountHash is " + tokenCountHash)

	return tokenLevelInt, tokenCountHash
}

func calcSHA256(targetHash string, maxNumber int) int {

	for i := 0; i < maxNumber; i++ {
		hash := sha256.Sum256([]byte(strconv.Itoa(i)))
		hashString := fmt.Sprintf("%x", hash)

		if strings.Compare(hashString, targetHash) == 0 {
			fmt.Println("Hash found at:", i)
			return i
		}
	}
	return -1
}

func maxTokenFromLevel(level int) int {

	val := TokenMap[level]
	//	fmt.Println("maxTokenFromLevel val is " + strconv.Itoa(val))
	return val
}

func (ta *TokenAuth) validateWholeToken(wholeTokenName string) (bool, string) {

	tokenLevel, tokenCountHash := ta.getWholeTokenValue(wholeTokenName)
	tokenLevelStatus := false
	validationStatus := false
	tokenVal := -1
	ta.log.Debug("tokenLevel from validateWholeToken is "+strconv.Itoa(tokenLevel), "debug")

	if _, ok := TokenMap[tokenLevel]; ok {
		tokenLevelStatus = true
		//	fmt.Println("Token Level is valid")
	} else {
		return false, "Token Level is invalid"
	}

	if tokenLevelStatus {
		tokenVal = calcSHA256(tokenCountHash, maxTokenFromLevel(tokenLevel))
	}

	ta.log.Info("Token Value is " + strconv.Itoa(tokenVal))

	if tokenLevel != -1 && tokenVal != -1 {
		validationStatus = true
	} else {
		return false, "Token Count is invalid"
	}
	return validationStatus, "Token is valid"

}
