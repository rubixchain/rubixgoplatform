package util

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
	"strings"

	"golang.org/x/crypto/sha3"
)

type RandPos struct {
	OriginalPos []int `json:"originalPos"`
	PosForSign  []int `json:"posForSign"`
}

func GetRandBytes(num int) []byte {
	d := make([]byte, num)
	rand.Read(d)
	return d
}

func GetRandString() string {
	d := make([]byte, 32)
	rand.Read(d)
	return HexToStr(d)
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

func CalculateHashString(msg string, method string) string {
	switch method {
	case "SHA3-256":
		h := sha3.New256()
		h.Write([]byte(msg))
		b := h.Sum(nil)
		return HexToStr(b)
	default:
		return ""
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

func IsFileExist(fileName string) bool {
	_, err := os.Stat(fileName)
	return err == nil
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

func GetAllFiles(root string) ([]string, error) {
	var files []string
	_, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	fs, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, f := range fs {
		if !f.IsDir() {
			files = append(files, f.Name())
		}
	}
	return files, nil
}

func RandomPositions(role string, hash string, numOfPositions int, pvt1 []int) *RandPos {
	var u, l, m int = 0, 0, 0

	hashCharacters := make([]int, 32)
	randomPositions := make([]int, 32)
	randPos := make([]int, 256)
	var finalPositions, pos []int
	originalPos := make([]int, 32)
	posForSign := make([]int, 32*8)

	for k := 0; k < numOfPositions; k++ {

		temp, err := strconv.ParseInt(string(hash[k]), 16, 32)
		if err != nil {
			return nil
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
		if role == "signer" {
			var p1 []int = GetPrivatePositions(finalPositions, pvt1)
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
	return &RandPos{
		OriginalPos: originalPos, PosForSign: posForSign}
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

func HexToStr(d []byte) string {
	dst := make([]byte, hex.EncodedLen(len(d)))
	hex.Encode(dst, d)

	return string(dst)
}

func StrToHex(s string) []byte {
	dst := make([]byte, hex.DecodedLen(len(s)))
	hex.Decode(dst, []byte(s))
	return dst
}

func ConvertBitString(b string) []byte {
	var out []byte
	var str string

	for i := len(b); i > 0; i -= 8 {
		if i-8 < 0 {
			str = string(b[0:i])
		} else {
			str = string(b[i-8 : i])
		}
		v, err := strconv.ParseUint(str, 2, 8)
		if err != nil {
			return nil
		}
		out = append([]byte{byte(v)}, out...)
	}
	return out
}

func ConvertToBitString(data []byte) string {
	var bits string = ""
	for i := 0; i < len(data); i++ {
		bits = bits + fmt.Sprintf("%08b", data[i])
	}
	return bits
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

func ConvertToJson(d interface{}) string {
	b, err := json.Marshal(d)
	if err != nil {
		return ""
	}
	return string(b)
}

func ConvertToJsonString(d interface{}) string {
	b, err := json.MarshalIndent(d, "", "   ")
	if err != nil {
		return ""
	}
	return string(b)
}

func BitstreamToBytes(stream string) ([]byte, error) {
	result := make([]byte, 0)
	str := stream
	for {
		l := 0
		if len(str) > 8 {
			l = len(str) - 8
		}
		temp, err := strconv.ParseInt(str[l:], 2, 64)
		if err != nil {
			return nil, err
		}
		result = append([]byte{byte(temp)}, result...)
		if l == 0 {
			break
		} else {
			str = str[:l]
		}
	}
	return result, nil
}

func BytesToBitstream(data []byte) string {
	var str string
	for _, d := range data {
		str = str + fmt.Sprintf("%08b", d)
	}
	return str
}

func CreateDir(dirPath string) error {
	return os.MkdirAll(dirPath, os.ModeDir|os.ModePerm)
}

func ParseJsonFile(fileName string, m interface{}) error {
	b, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, m)
}

func marshal(str string, m interface{}, keys []string) (string, error) {
	var err error
	switch mt := m.(type) {
	case []map[string]interface{}:
		str = str + "["
		c1 := false
		for i := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str = str + "{"
			firstElem := true
			for _, k := range keys {
				v, ok := mt[i][k]
				if ok {
					if !firstElem {
						str = str + ","
					}
					firstElem = false
					str = str + "\"" + k + "\":"
					s, ok := v.(string)
					if ok {
						str = str + "\"" + s + "\""
					} else {
						if k == "distributedObject" {
							str, err = marshal(str, v, nil)
							if err != nil {
								return "", err
							}
						} else {
							str, err = marshal(str, v, []string{"node", "tokens"})
							if err != nil {
								return "", err
							}
						}

					}
				}
			}
			str = str + "}"
		}
		str = str + "]"
	case map[string]interface{}:
		str = str + "{"
		c1 := false
		if keys == nil {
			for k, v := range mt {
				if c1 {
					str = str + ","
				}
				c1 = true
				str = str + "\"" + k + "\":"
				s, ok := v.(string)
				if ok {
					str = str + "\"" + s + "\""
				} else {
					str, err = marshal(str, v, keys)
					if err != nil {
						return "", err
					}
				}
			}
		} else {
			for _, k := range keys {
				v, ok := mt[k]
				if ok {
					if c1 {
						str = str + ","
					}
					c1 = true
					str = str + "\"" + k + "\":"
					s, ok := v.(string)
					if ok {
						str = str + "\"" + s + "\""
					} else {
						str, err = marshal(str, v, keys)
						if err != nil {
							return "", err
						}
					}
				}
			}
		}
		str = str + "}"
	case []interface{}:
		str = str + "["
		c1 := false
		for _, mf := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			s, ok := mf.(string)
			if ok {
				str = str + "\"" + s + "\""
			} else {
				str, err = marshal(str, mf, keys)
				if err != nil {
					return "", err
				}
			}
		}
		str = str + "]"
	default:
		return "", fmt.Errorf("invalid type %T", mt)
	}
	return str, nil
}

func CalcTokenChainHash(tc []map[string]interface{}) string {
	l := len(tc)
	if l == 0 {
		return ""
	}
	_, ok := tc[l-1]["hash"]
	if ok {
		delete(tc[l-1], "hash")
	}
	_, ok = tc[l-1]["pvtShareBits"]
	if ok {
		delete(tc[l-1], "pvtShareBits")
	}

	keys := []string{"owner", "tokensPledgedWith", "tokensPledgedFor", "receiver", "sender", "senderSign", "comment", "distributedObject", "tid", "pledgeToken", "hash", "group", "pvtShareBits"}

	var err error
	str := ""
	str, err = marshal(str, tc, keys)
	if err != nil {
		fmt.Printf("Failed %s", err.Error())
		return ""
	}

	// return str

	// str := "["
	// c1 := false
	// for i := range tc {
	// 	if c1 {
	// 		str = str + ","
	// 	}
	// 	c1 = true
	// 	str = str + "{"
	// 	firstElem := true
	// 	for _, k := range keys {
	// 		v, ok := tc[i][k]
	// 		if ok {
	// 			if !firstElem {
	// 				str = str + ","
	// 			}
	// 			firstElem = false
	// 			str = str + "\"" + k + "\":"
	// 			s, ok := v.(string)
	// 			if ok {
	// 				str = str + "\"" + s + "\""
	// 			} else {
	// 				sa, ok := v.([]interface{})
	// 				if ok {
	// 					str = str + "["
	// 					comma := false
	// 					for _, s := range sa {
	// 						if comma {
	// 							str = str + ","
	// 						}
	// 						comma = true
	// 						str = str + "\"" + s.(string) + "\""
	// 					}
	// 					str = str + "]"
	// 				}
	// 			}
	// 		}
	// 	}
	// 	str = str + "}"
	// }
	// str = str + "]"
	return str
}

func GetFromMap(m interface{}, key string) interface{} {
	switch mm := m.(type) {
	case map[string]interface{}:
		return mm[key]
	case map[interface{}]interface{}:
		return mm[key]
	}
	return nil
}

func GetStringFromMap(m interface{}, key string) string {
	var si interface{}
	switch mm := m.(type) {
	case map[string]interface{}:
		si = mm[key]
	case map[interface{}]interface{}:
		si = mm[key]
	default:
		return ""
	}
	switch s := si.(type) {
	case string:
		return s
	case interface{}:
		str, ok := si.(string)
		if ok {
			return str
		}
	}
	return ""
}

func GetStringSliceFromMap(m interface{}, key string) []string {
	var si interface{}
	switch mm := m.(type) {
	case map[string]interface{}:
		si = mm[key]
	case map[interface{}]interface{}:
		si = mm[key]
	default:
		return nil
	}
	switch s := si.(type) {
	case []string:
		return s
	case interface{}:
		str, ok := si.([]string)
		if ok {
			return str
		}
	}
	return nil
}

func GetInt(si interface{}) int {
	var tl int
	switch mt := si.(type) {
	case int:
		tl = mt
	case int64:
		tl = int(mt)
	case uint64:
		tl = int(mt)
	default:
		tl = 0
	}
	return tl
}

func GetBytes(si interface{}) []byte {
	switch s := si.(type) {
	case []byte:
		return s
	case interface{}:
		st, ok := s.([]byte)
		if ok {
			return st
		}
	}
	return nil
}

func GetString(si interface{}) string {
	switch s := si.(type) {
	case string:
		return s
	case interface{}:
		st, ok := s.(string)
		if ok {
			return st
		}
	}
	return ""
}

func GetIntFromMap(m interface{}, key string) int {
	var tli interface{}
	var ok bool
	switch mm := m.(type) {
	case map[string]interface{}:
		tli, ok = mm[key]
		if !ok {
			return 0
		}
	case map[interface{}]interface{}:
		tli, ok = mm[key]
		if !ok {
			return 0
		}
	default:
		return 0
	}
	var tl int
	switch mt := tli.(type) {
	case int:
		tl = mt
	case int64:
		tl = int(mt)
	case uint64:
		tl = int(mt)
	default:
		tl = 0
	}
	return tl
}

func GetFloatFromMap(m interface{}, key string) float64 {
	var tli interface{}
	var ok bool
	switch mm := m.(type) {
	case map[string]interface{}:
		tli, ok = mm[key]
		if !ok {
			return 0
		}
	case map[interface{}]interface{}:
		tli, ok = mm[key]
		if !ok {
			return 0
		}
	default:
		return 0
	}
	var tl float64
	switch mt := tli.(type) {
	case float64:
		tl = mt
	case float32:
		tl = float64(mt)
	case int:
		tl = float64(mt)
	case int64:
		tl = float64(mt)
	case uint64:
		tl = float64(mt)
	default:
		tl = 0
	}
	return tl
}

func RemoveAtIndex(slice []string, index int) []string {
	return append(slice[:index], slice[index+1:]...)
}

func BytesToString(b []byte) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
