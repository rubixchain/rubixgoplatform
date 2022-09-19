package util

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"path"
	"strings"

	"golang.org/x/crypto/sha3"
)

type RandPosObj struct {
	OriginalPos []int `json:"originalPos"`
	PosForSign  []int `json:"posForSign"`
}

func CalculateHash(data []byte, method string) []byte {
	switch method {
	case "SHA3-256":
		h := sha3.New256()
		h.Write(data)
		return h.Sum(nil)
	default:
		return nil
	}
}

func GetMACAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, i := range interfaces {
		ha := i.HardwareAddr
		return fmt.Sprintf("%x", ha)
	}
	return ""
}

func Filecopy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

func FileWrite(fileName string, data []byte) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func RandomPositions(role string, hash string, numOfPositions int, pvt1 []int) ([]byte, error) {
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
			hash = HexToStr(CalculateHash([]byte(hash+IntArraytoStr(originalPos)+IntArraytoStr(p1)), "SHA3-256"))

		} else {
			p1 := make([]int, 8)

			for i := 0; i < 8; i++ {
				p1[i] = pvt1[m]
				m++
			}
			hash = HexToStr(CalculateHash([]byte(hash+IntArraytoStr(originalPos)+IntArraytoStr(p1)), "SHA3-256"))

		}
	}
	result := RandPosObj{
		OriginalPos: originalPos, PosForSign: posForSign}

	result_obj, err := json.Marshal(result)

	if err != nil {
		var emptyresult []byte
		return emptyresult, err
	}

	return result_obj, err
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
func DirCopy(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			err = os.MkdirAll(dstfp, srcinfo.Mode())
			if err != nil {
				return err
			}
		} else {
			if _, err = Filecopy(srcfp, dstfp); err != nil {
				return err
			}
		}
	}
	return nil
}

// SanitizeDirPath will check for proper directory path
func SanitizeDirPath(path string) string {
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		return path
	} else {
		return path + "/"
	}
}

// ParseAddress will parse the addrees and split inot Peer ID  & DID
func ParseAddress(addr string) (string, string, bool) {
	peerID := ""
	did := ""
	// check if addr contains the peer ID
	if strings.Contains(addr, ".") {
		str := strings.Split(addr, ".")
		if len(str) != 2 {
			return "", "", false
		}
		peerID = str[0]
		did = str[1]
	} else {
		did = addr
	}
	//TODO:: Validation
	return peerID, did, true
}

// CreateAddress will create the addrees fromt Peer ID  & DID
func CreateAddress(peerID string, did string) string {
	return peerID + "." + did
}
