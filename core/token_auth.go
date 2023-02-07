package core

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

func (c *Core) getWholeTokenValue(wholeTokenName string) (int, string) {
	response, err := c.ipfs.Cat(wholeTokenName)
	if err != nil {
		c.log.Error("Error occured while fetching token content", "error", err)
		return -1, ""
	}

	result, err := ioutil.ReadAll(response)

	if err != nil {
		c.log.Error("Error occured", "error", err)
		return -1, ""
	}

	trimmedResult := strings.TrimSpace(string(result))

	response.Close()

	tokenLevel := string(trimmedResult[:len(trimmedResult)-64])
	tokenCountHash := string(trimmedResult[len(trimmedResult)-64:])

	fmt.Println(len(tokenCountHash), "token len")

	tokenLevelInt, err := strconv.Atoi(tokenLevel)
	if err != nil {
		fmt.Println("Error occured", "error", err)
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
			fmt.Println("Found:", i)
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

func (c *Core) validateWholeToken(wholeTokenName string) (bool, string) {

	tokenLevel, tokenCountHash := c.getWholeTokenValue(wholeTokenName)
	tokenLevelStatus := false
	validationStatus := false
	tokenVal := -1
	fmt.Println("tokenLevel from validateWholeToken is " + strconv.Itoa(tokenLevel))

	if _, ok := TokenMap[tokenLevel]; ok {
		tokenLevelStatus = true
		//	fmt.Println("Token Level is valid")
	} else {
		return false, "Token Level is invalid"
	}

	if tokenLevelStatus {
		tokenVal = calcSHA256(tokenCountHash, maxTokenFromLevel(tokenLevel))
	}

	c.log.Info("Token Value is " + strconv.Itoa(tokenVal))

	if tokenLevel != -1 {
		validationStatus = true
	} else {
		return false, "Token Count is invalid"
	}
	return validationStatus, "Token is valid"

}
