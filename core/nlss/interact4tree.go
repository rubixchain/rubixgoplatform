package nlss

import (
	"math"
)

// ShareType ...
type ShareType byte

// ShareMode ...
type ShareMode byte

// Sharetype
const (
	PrivateShare   ShareType = 0
	EssentialShare           = 1
	PublicShare              = 2
)

// ShareMode
const (
	ShareTwo1x2  ShareMode = 0
	ShareFour1x4           = 1
	ShareFour1x6           = 2
	ShareFour1x9           = 3
)

// ShareFormat ...
type ShareFormat struct {
	ShareID    byte
	Type       ShareType
	UID        string
	ShareBytes []byte
}

// Shares ...
type Shares struct {
	NumShares byte
	Mode      ShareMode
	Share     map[byte]*ShareFormat
}

// Interact4Tree ...
type Interact4Tree struct {
	secretString string
	y1String     string
	//	y2String string
	strs  string
	bits  string
	pvt   string
	cnd   string
	cnd1  string
	cand1 [][]int
	//	cand2        [][]int
	secret [][]int
}

// GetNumShares ...
func GetNumShares(mode ShareMode) byte {
	switch mode {
	case ShareTwo1x2:
		return 2
	case ShareFour1x4:
		return 4
	case ShareFour1x6:
		return 6
	case ShareFour1x9:
		return 9
	}
	return 0
}

// ConvertString ...
func ConvertString(str string, s int) string {
	if s == 1 {
		str = str + "1"
	} else {
		str = str + "0"
	}
	return str
}

// NewInteract4Tree ...
func NewInteract4Tree(data []byte) *Interact4Tree {

	bits := ConvertToBitString(data)
	it := &Interact4Tree{
		bits: bits,
	}

	it.cand1 = make([][]int, len(bits))
	//	it.cand2 = make([][]int, len(bits))
	it.secret = make([][]int, len(bits))
	return it
}

// ShareCreate ...
func (it *Interact4Tree) ShareCreate() bool {
	for i := 0; i < len(it.bits); i++ {
		var share *SecretShare
		if it.bits[i] == '0' {
			share = NewSecretShare(0)
		} else {
			share = NewSecretShare(1)
		}
		share.Starts()
		for j := 0; j < 8; j++ {
			it.secret[i] = append(it.secret[i], share.s0[j])
			it.cand1[i] = append(it.cand1[i], share.y1[j])
			//it.cand2[i] = append(it.cand2[i], share.y2[j])
			it.secretString = ConvertString(it.secretString, share.s0[j])
			it.y1String = ConvertString(it.y1String, share.y1[j])
			//	it.y2String = ConvertString(it.y2String, share.y2[j])
		}
	}
	return it.CheckShare()
}

// CheckShare ...
func (it *Interact4Tree) CheckShare() bool {
	var verifies bool
	var sum1 int
	//var sum2 int
	verifies = true
	for i := 0; i < len(it.secret); i++ {
		sum1 = 0
		//sum2 = 0
		for j := 0; j < len(it.secret[i]); j++ {
			sum1 = sum1 + (it.secret[i][j] * it.cand1[i][j])
			//sum2 = sum2 + (it.secret[i][j] * it.cand2[i][j])
		}
		sum1 = sum1 % 2
		//sum2 = sum2 % 2
		if sum1 != (int(it.bits[i]) - 48) {
			verifies = false
		}
	}
	return verifies
}

// GetPvtShare ...
func (it *Interact4Tree) GetPvtShare() []byte {
	if len(it.secretString) == 0 {
		return nil
	}
	if len(it.secretString)%8 != 0 {
		return nil
	}
	return ConvertBitString(it.secretString)
}

// GetPublicShare1 ...
func (it *Interact4Tree) GetPublicShare1() []byte {
	if len(it.y1String) == 0 {
		return nil
	}
	if len(it.y1String)%8 != 0 {
		return nil
	}
	return ConvertBitString(it.y1String)
}

// // GetPublicShare2 ...
func (it *Interact4Tree) GetPublicShare2() []byte {
	if len(it.y1String) == 0 {
		return nil
	}
	if len(it.y1String)%8 != 0 {
		return nil
	}
	return ConvertBitString(it.y1String)
}

// Gen3Shares ...
func Gen3Shares(data []byte) ([]byte, []byte, []byte) {
	it := NewInteract4Tree(data)
	if it.ShareCreate() == false {
		return nil, nil, nil
	}
	pvt := it.GetPvtShare()
	pub1 := it.GetPublicShare1()
	pub2 := it.GetPublicShare2()
	return pvt, pub1, pub2
}

func Gen2Shares(data []byte) ([]byte, []byte) {
	it := NewInteract4Tree(data)
	if it.ShareCreate() == false {
		return nil, nil
	}
	pvt := it.GetPvtShare()
	pub1 := it.GetPublicShare1()
	//	pub2 := it.GetPublicShare2()
	return pvt, pub1
}

// GenShares ...
func GenShares(data []byte, mode ShareMode) *Shares {
	temp1, temp2 := Gen2Shares(data)
	if mode == ShareTwo1x2 {
		s1 := &ShareFormat{
			ShareID:    0,
			Type:       PrivateShare,
			ShareBytes: temp1,
		}
		s2 := &ShareFormat{
			ShareID:    1,
			Type:       PrivateShare,
			ShareBytes: temp2,
		}
		s := &Shares{
			NumShares: 2,
			Mode:      mode,
			Share:     make(map[byte]*ShareFormat, 2),
		}
		s.Share[0] = s1
		s.Share[1] = s2
		return s
	}
	// pvt, pub11, pub12 := Gen3Shares(temp1)
	// ess1, pub21, pub22 := Gen3Shares(temp2)
	// if mode == ShareFour1x4 {
	// 	s1 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PrivateShare,
	// 		ShareBytes: pvt,
	// 	}
	// 	s2 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PrivateShare,
	// 		ShareBytes: pub11,
	// 	}
	// 	s3 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       PrivateShare,
	// 		ShareBytes: ess1,
	// 	}
	// 	s4 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       PrivateShare,
	// 		ShareBytes: pub21,
	// 	}
	// 	s := &Shares{
	// 		NumShares: 4,
	// 		Mode:      mode,
	// 		Share:     make(map[byte]*ShareFormat, 4),
	// 	}
	// 	s.Share[0] = s1
	// 	s.Share[1] = s2
	// 	s.Share[2] = s3
	// 	s.Share[3] = s4
	// 	return s
	// }
	// if mode == ShareFour1x6 {
	// 	s1 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PrivateShare,
	// 		ShareBytes: pvt,
	// 	}
	// 	s2 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub11,
	// 	}
	// 	s3 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub12,
	// 	}
	// 	s4 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       PrivateShare,
	// 		ShareBytes: ess1,
	// 	}
	// 	s5 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub21,
	// 	}
	// 	s6 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub22,
	// 	}
	// 	s := &Shares{
	// 		NumShares: 6,
	// 		Mode:      mode,
	// 		Share:     make(map[byte]*ShareFormat, 6),
	// 	}
	// 	s.Share[0] = s1
	// 	s.Share[1] = s2
	// 	s.Share[2] = s3
	// 	s.Share[3] = s4
	// 	s.Share[4] = s5
	// 	s.Share[5] = s6
	// 	return s
	// }
	// ess2, pub31, pub32 := Gen3Shares(temp3)
	// if mode == ShareFour1x9 {
	// 	s1 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PrivateShare,
	// 		ShareBytes: pvt,
	// 	}
	// 	s2 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub11,
	// 	}
	// 	s3 := &ShareFormat{
	// 		ShareID:    0,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub12,
	// 	}
	// 	s4 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       EssentialShare,
	// 		ShareBytes: ess1,
	// 	}
	// 	s5 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub21,
	// 	}
	// 	s6 := &ShareFormat{
	// 		ShareID:    1,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub22,
	// 	}
	// 	s7 := &ShareFormat{
	// 		ShareID:    2,
	// 		Type:       EssentialShare,
	// 		ShareBytes: ess2,
	// 	}
	// 	s8 := &ShareFormat{
	// 		ShareID:    2,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub31,
	// 	}
	// 	s9 := &ShareFormat{
	// 		ShareID:    2,
	// 		Type:       PublicShare,
	// 		ShareBytes: pub32,
	// 	}
	// 	s := &Shares{
	// 		NumShares: 9,
	// 		Mode:      mode,
	// 		Share:     make(map[byte]*ShareFormat, 9),
	// 	}
	// 	s.Share[0] = s1
	// 	s.Share[1] = s2
	// 	s.Share[2] = s3
	// 	s.Share[3] = s4
	// 	s.Share[4] = s5
	// 	s.Share[5] = s6
	// 	s.Share[6] = s7
	// 	s.Share[7] = s8
	// 	s.Share[8] = s9
	// 	return s
	// }
	return nil
}

// Combine2Shares ...
func Combine2Shares(pvt []byte, pub []byte) []byte {
	pvtString := ConvertToBitString(pvt)
	pubString := ConvertToBitString(pub)
	if len(pvtString) != len(pubString) {
		return nil
	}
	var sum int
	var temp string = ""
	for i := 0; i < len(pvtString); i = i + 8 {
		sum = 0
		for j := i; j < i+8; j++ {
			sum = sum + (int(pvtString[j]-0x30) * int(pubString[j]-0x30))
		}
		sum = sum % 2
		temp = ConvertString(temp, sum)
	}
	return (ConvertBitString(temp))
}

// IsShareFound ...
func IsShareFound(s *Shares, id byte, t ShareType) (bool, int) {
	var i byte
	var found bool
	var count int
	found = false
	count = 0
	for i = 0; i < s.NumShares; i++ {
		if s.Share[i].ShareID == id && s.Share[i].Type == t {
			found = true
			count++
		}
	}
	return found, count
}

// GetShareBytes ...
func GetShareBytes(s *Shares, id byte, t ShareType, index int) []byte {
	var i byte
	var count int
	count = 0
	for i = 0; i < s.NumShares; i++ {
		if s.Share[i].ShareID == id && s.Share[i].Type == t {
			if count == index {
				return s.Share[i].ShareBytes
			}
			count++
		}
	}
	return nil
}

// CombineShares ...
func CombineShares(s *Shares) []byte {
	if s.Mode == ShareTwo1x2 {
		if s.NumShares != 2 {
			return nil
		}
		if found, _ := IsShareFound(s, 0, PrivateShare); found == false {
			return nil
		}
		if found, _ := IsShareFound(s, 1, PrivateShare); found == false {
			return nil
		}
		return Combine2Shares(s.Share[0].ShareBytes, s.Share[1].ShareBytes)
	}
	if s.Mode == ShareFour1x4 {
		if s.NumShares != 4 {
			return nil
		}
		if found, count := IsShareFound(s, 0, PrivateShare); found == false || count < 2 {
			return nil
		}
		if found, count := IsShareFound(s, 1, PrivateShare); found == false || count < 2 {
			return nil
		}
		temp1 := Combine2Shares(GetShareBytes(s, 0, PrivateShare, 0), GetShareBytes(s, 0, PrivateShare, 1))
		temp2 := Combine2Shares(GetShareBytes(s, 1, PrivateShare, 0), GetShareBytes(s, 1, PrivateShare, 1))
		return Combine2Shares(temp1, temp2)
	}
	if s.Mode == ShareFour1x6 {
		if s.NumShares < 4 {
			return nil
		}
		if found, _ := IsShareFound(s, 0, PrivateShare); found == false {
			return nil
		}
		if found, _ := IsShareFound(s, 0, PublicShare); found == false {
			return nil
		}
		if found, _ := IsShareFound(s, 1, PrivateShare); found == false {
			return nil
		}
		if found, _ := IsShareFound(s, 1, PublicShare); found == false {
			return nil
		}
		temp1 := Combine2Shares(GetShareBytes(s, 0, PrivateShare, 0), GetShareBytes(s, 0, PublicShare, 0))
		temp2 := Combine2Shares(GetShareBytes(s, 1, PrivateShare, 0), GetShareBytes(s, 1, PublicShare, 0))
		return Combine2Shares(temp1, temp2)
	}
	if s.Mode == ShareFour1x9 {
		if s.NumShares < 4 {
			return nil
		}
		if found, _ := IsShareFound(s, 0, PrivateShare); found == false {
			return nil
		}
		if found, _ := IsShareFound(s, 0, PublicShare); found == false {
			return nil
		}
		var id byte
		var essFound bool
		essFound = false
		if found, _ := IsShareFound(s, 1, EssentialShare); found == true {

			if found, _ := IsShareFound(s, 1, PublicShare); found == false {
				return nil
			}
			essFound = true
			id = 0
		}
		if essFound == false {
			if found, _ := IsShareFound(s, 2, EssentialShare); found == true {
				if found, _ := IsShareFound(s, 2, PublicShare); found == false {
					return nil
				}
				essFound = true
				id = 1
			}
		}
		if essFound == false {
			return nil
		}
		temp1 := Combine2Shares(GetShareBytes(s, 0, PrivateShare, 0), GetShareBytes(s, 0, PublicShare, 0))
		temp2 := Combine2Shares(GetShareBytes(s, id, EssentialShare, 0), GetShareBytes(s, id, PublicShare, 0))
		return Combine2Shares(temp1, temp2)
	}
	return nil
}

// GetRandomCoord ...
func GetRandomCoord(data []byte, level byte, numCoord int, coord []int) ([]byte, []int) {
	var temp []int
	dataString := ConvertToBitString(data)
	if coord == nil && level != 0 {
		return nil, nil
	}
	if coord == nil {
		if numCoord > len(data)*8 {
			return nil, nil
		}
		for i := 0; i < numCoord; i++ {
			temp = append(temp, GetRandNumber(len(data)*8-1))
		}
	} else {
		temp = coord
	}
	var tempString string = ""
	for i := 0; i < numCoord; i++ {
		numBits := int(math.Pow(8, float64(level)))
		bitPos := temp[i] * numBits
		if bitPos+numBits > len(dataString) {
			return nil, nil
		}
		tempString = tempString + dataString[bitPos:bitPos+numBits]
	}
	dataOut := ConvertBitString(tempString)
	return dataOut, temp
}
