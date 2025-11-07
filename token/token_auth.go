package token

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

const (
	RBTTokenType  = 0
	PartTokenType = 1
	NFTTokenType  = 2
	TestTokenType = 3
	// Data Token Type Removed
	TestPartTokenType = 5
	TestNFTTokenType  = 6
	// Test Data token type removed
	SmartContractTokenType     = 8
	TestSmartContractTokenType = 9
	FTTokenType                = 10
)

func GetWholeTokenValue(tokenDetails string) (int, string, bool, error) {
	trimmedResult := strings.TrimSpace(tokenDetails)
	tokenLevel := string(trimmedResult[:len(trimmedResult)-64])
	tokenCountHash := string(trimmedResult[len(trimmedResult)-64:])
	tokenLevelInt, err := strconv.Atoi(tokenLevel)
	if err != nil {
		return 0, "", false, err
	}
	needMigration := false
	if len(tokenLevel) < 3 {
		if tokenLevelInt != 1 {
			return 0, "", false, fmt.Errorf("invalid token level format")
		}
		needMigration = true
	}
	return tokenLevelInt, tokenCountHash, needMigration, nil
}

func CheckWholeToken(tokenDetails string, testNet bool) (string, bool, error) {
	isWholeToken := true
	trimmedResult := strings.Split(strings.TrimSpace(tokenDetails), ",")
	if testNet && len(trimmedResult) == 3 {
		return "", !isWholeToken, nil
	} else {
		trimmedResultVal := trimmedResult[0]
		tokenLevel := string(trimmedResultVal[:len(trimmedResultVal)-64])
		// tokenCountHash := string(trimmedResultVal[len(trimmedResult)-64:])
		tokenLevelInt, err := strconv.Atoi(tokenLevel) //It will always be 0
		if err != nil {
			return "", !isWholeToken, err
		}
		if len(tokenLevel) < 3 {
			if tokenLevelInt != 1 {
				return "", !isWholeToken, fmt.Errorf("invalid token level format")
			}
		}
	}
	return "", isWholeToken, nil

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

func MaxTokenFromLevel(level int) int {
	val := TokenMap[level]
	return val
}

func ValidateTokenDetials(tl int, tn int) bool {
	if tn < 0 {
		return false
	}
	return tn < TokenMap[tl]
}

func ValidateWholeToken(tokenDetials string) (int, int, bool, error) {
	tokenLevel, tokenCountHash, needMigration, err := GetWholeTokenValue(tokenDetials)
	if err != nil {
		return -1, -1, false, err
	}
	tokenVal := -1
	tokenVal = calcSHA256(tokenCountHash, MaxTokenFromLevel(tokenLevel))
	if tokenVal == -1 {
		return -1, -1, false, fmt.Errorf("token Count is invalid")
	}
	return tokenLevel, tokenVal, needMigration, nil
}

func GetTokenString(tl int, tn int) string {
	str := ""
	str = fmt.Sprintf("%03d", tl)
	hash := sha256.Sum256([]byte(strconv.Itoa(tn)))
	str = str + fmt.Sprintf("%x", hash)
	return str
}

func GetLevelOneTokenString(tl int, tn int) string {
	str := ""
	if tl == 1 {
		str = fmt.Sprintf("%02d", tl)
	} else {
		str = fmt.Sprintf("%03d", tl)
	}
	hash := sha256.Sum256([]byte(strconv.Itoa(tn)))
	str = str + fmt.Sprintf("%x", hash)
	return str
}
