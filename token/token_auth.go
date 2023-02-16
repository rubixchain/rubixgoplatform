package token

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

func getWholeTokenValue(tokenDetials string) (int, string, error) {

	trimmedResult := strings.TrimSpace(tokenDetials)

	tokenLevel := string(trimmedResult[:len(trimmedResult)-64])
	tokenCountHash := string(trimmedResult[len(trimmedResult)-64:])
	tokenLevelInt, err := strconv.Atoi(tokenLevel)
	if err != nil {
		return 0, "", err
	}
	return tokenLevelInt, tokenCountHash, nil
}

func calcSHA256(targetHash string, maxNumber int) int {

	for i := 0; i < maxNumber; i++ {
		hash := sha256.Sum256([]byte(strconv.Itoa(i)))
		hashString := fmt.Sprintf("%x", hash)
		if strings.Compare(hashString, targetHash) == 0 {
			return i
		}
	}
	return -1
}

func maxTokenFromLevel(level int) int {

	val := TokenMap[level]
	return val
}

func ValidateWholeToken(tokenDetials string) (int, int, error) {
	tokenLevel, tokenCountHash, err := getWholeTokenValue(tokenDetials)
	if err != nil {
		return -1, -1, err
	}
	tokenVal := -1

	if _, ok := TokenMap[tokenLevel]; !ok {
		return -1, -1, fmt.Errorf("token Level is invalid")
	}
	tokenVal = calcSHA256(tokenCountHash, maxTokenFromLevel(tokenLevel))

	if tokenVal == -1 {
		return -1, -1, fmt.Errorf("token Count is invalid")
	}
	return tokenLevel, tokenVal, nil
}
