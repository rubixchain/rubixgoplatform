package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	prfSha1   uint = 0
	prfSha256 uint = 1
	prfSha512 uint = 2
)

const (
	kdfVersion3 byte = 3
)

// HashPassword ...
func HashPassword(password string, v byte, prf uint, count uint) string {
	switch v {
	case 1:
		return hashPasswordV1(password, prf, count)
	case 3:
		return hashPasswordV3(password, prf, count)
	default:
		return ""
	}
}

// VerifyPassword ..
func VerifyPassword(password string, paaswordHash string) bool {
	if strings.Contains(paaswordHash, "$MYHASH$V1") {
		return verifyPasswordV1(password, paaswordHash)
	}
	pwd, err := base64.StdEncoding.DecodeString(paaswordHash)
	if err != nil {
		return false
	}
	if pwd[0] == 0x01 {
		return verifyPasswordV3(password, pwd)
	} else {
		return false
	}
}

// VerifyPasswordV3 ..
func verifyPasswordV1(password string, passswordHash string) bool {
	tempStr := strings.ReplaceAll(passswordHash, "$MYHASH$V1$", "")

	splitStr := strings.Split(tempStr, "$")

	count, err := strconv.ParseInt(splitStr[0], 10, 64)

	if err != nil {
		return false
	}

	hashBytes, err := base64.StdEncoding.DecodeString(splitStr[1])

	if err != nil {
		return false
	}

	if len(hashBytes) < 16 {
		return false
	}

	salt := hashBytes[:16]

	hashSize := len(hashBytes) - 16

	var prf int

	switch hashSize {
	case 20:
		prf = 0
	case 32:
		prf = 1
	case 64:
		prf = 2
	default:
		return false
	}

	var subkey []byte
	if prf == 0 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 20, sha1.New)
	} else if prf == 1 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha256.New)
	} else {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 64, sha512.New)
	}
	if bytes.Equal(subkey, hashBytes[16:]) {
		return true
	} else {
		return false
	}
}

// VerifyPasswordV3 ..
func verifyPasswordV3(password string, pwd []byte) bool {
	prf := ReadNetworkOrder(pwd, 1)
	count := ReadNetworkOrder(pwd, 5)
	sl := ReadNetworkOrder(pwd, 9)

	salt := pwd[13:(13 + sl)]
	var subkey []byte
	if prf == 0 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha1.New)
	} else if prf == 1 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha256.New)
	} else {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha512.New)
	}
	if bytes.Compare(subkey, pwd[(13+sl):]) == 0 {
		return true
	} else {
		return false
	}
}

// hashPasswordV1 ..
func hashPasswordV1(password string, prf uint, count uint) string {
	salt := make([]byte, 16)
	io.ReadFull(rand.Reader, salt[:])
	var subkey []byte
	if prf == 0 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 20, sha1.New)
	} else if prf == 1 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha256.New)
	} else {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 64, sha512.New)
	}
	result := make([]byte, len(salt)+len(subkey))
	copy(result[0:], salt)
	copy(result[(len(salt)):], subkey)
	hashStr := fmt.Sprintf("$MYHASH$V1$%d$%s", count, base64.StdEncoding.EncodeToString(result))
	return hashStr
}

// hashPasswordV3 ..
func hashPasswordV3(password string, prf uint, count uint) string {
	salt := make([]byte, 16)
	io.ReadFull(rand.Reader, salt[:])
	var subkey []byte
	if prf == 0 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha1.New)
	} else if prf == 1 {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha256.New)
	} else {
		subkey = pbkdf2.Key([]byte(password), salt, int(count), 32, sha512.New)
	}
	fmt.Println(salt)
	result := make([]byte, 13+len(salt)+len(subkey))
	result[0] = 0x01
	WriteNetworkOrder(result, 1, prf)
	WriteNetworkOrder(result, 5, count)
	WriteNetworkOrder(result, 9, uint(len(salt)))
	copy(result[13:], salt)
	copy(result[(13+len(salt)):], subkey)
	return base64.StdEncoding.EncodeToString(result)
}

// ReadNetworkOrder ..
func ReadNetworkOrder(data []byte, offset int) uint {
	return uint(data[offset])<<24 | uint(data[offset+1])<<16 | uint(data[offset+2])<<8 | uint(data[offset+3])
}

// WriteNetworkOrder ..
func WriteNetworkOrder(data []byte, offset int, value uint) {
	data[offset] = byte(value >> 24)
	data[offset+1] = byte(value >> 16)
	data[offset+2] = byte(value >> 8)
	data[offset+3] = byte(value)
}
