package wallet

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/EnsurityTechnologies/enscrypt"
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/util"
)

type CreditSignature struct {
	Signature     string `json:"signature"`
	PrivSingature string `json:"priv_signature"`
	DID           string `json:"did"`
	Hash          string `json:"hash"`
}

// DIDDummy will handle basic DID
type DIDDummy struct {
	did    string
	pvtKey []byte
	pubKey []byte
}

// InitDIDBasic will return the basic did handle
func InitDIDDummy(did string) *DIDDummy {
	pvtKey, pubKey, err := enscrypt.GenerateKeyPair(&enscrypt.CryptoConfig{Alg: enscrypt.ECDSAP256})
	if err != nil {
		return nil
	}
	return &DIDDummy{did: did, pvtKey: pvtKey, pubKey: pubKey}
}

// Sign will return the singature of the DID
func (d *DIDDummy) Sign(hash string) ([]byte, []byte, error) {

	PrivateKey, _, err := enscrypt.DecodeKeyPair("", d.pvtKey, nil)
	if err != nil {
		return nil, nil, err
	}
	rb := util.GetRandBytes(32)
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(hash+d.did+util.HexToStr(rb)), "SHA3-256"))
	pvtKeySign, err := enscrypt.Sign(PrivateKey, []byte(hashPvtSign))
	if err != nil {
		return nil, nil, err
	}
	return rb, pvtKeySign, err
}

// Sign will verifyt he signature
func (d *DIDDummy) Verify(hash string, pvtShareSig []byte, pvtKeySIg []byte) (bool, error) {
	// read senderDID

	_, pubKeyByte, err := enscrypt.DecodeKeyPair("", nil, d.pubKey)
	if err != nil {
		return false, err
	}
	hashPvtSign := util.HexToStr(util.CalculateHash([]byte(hash+d.did+util.HexToStr(pvtShareSig)), "SHA3-256"))
	if !enscrypt.Verify(pubKeyByte, []byte(hashPvtSign), pvtKeySIg) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}

func (d *DIDDummy) PvtSign(hash []byte) ([]byte, error) {

	PrivateKey, _, err := enscrypt.DecodeKeyPair("", d.pvtKey, nil)
	if err != nil {
		return nil, err
	}
	pvtKeySign, err := enscrypt.Sign(PrivateKey, hash)
	if err != nil {
		return nil, err
	}
	return pvtKeySign, nil
}
func (d *DIDDummy) PvtVerify(hash []byte, sign []byte) (bool, error) {

	_, pubKeyByte, err := enscrypt.DecodeKeyPair("", nil, d.pubKey)
	if err != nil {
		return false, err
	}
	if !enscrypt.Verify(pubKeyByte, hash, sign) {
		return false, fmt.Errorf("failed to verify private key singature")
	}
	return true, nil
}

func TestTemp(t *testing.T) {

	var tc []map[string]interface{}
	err := util.ParseJsonFile("d:/test.json", &tc)
	if err != nil {
		t.Fatalf("Failed to parse file, %s", err.Error())
	}
	l := len(tc)
	if l == 0 {
		t.Fatalf("Invalid array")
	}
	delete(tc[l-1], "hash")
	delete(tc[l-1], "pvtShareBits")
	str := util.CalcTokenChainHash(tc)
	util.FileWrite("d:/test_temp.json", []byte(str))
	hash := util.CalculateHash([]byte(str), "SHA3-256")
	if hash != nil {
		t.Fatalf("Failed to parse file, %s", err.Error())
	}
}

func TestTokenChainBlock(t *testing.T) {
	wc := &WalletConfig{
		StorageType:   storage.StorageDBType,
		DBAddress:     "wallet.db",
		DBType:        "Sqlite3",
		TokenChainDir: "./",
	}
	logOptions := &logger.LoggerOptions{
		Name:   "WalletTest",
		Color:  []logger.ColorOption{logger.AutoColor},
		Output: []io.Writer{logger.DefaultOutput},
	}

	log := logger.New(logOptions)
	w, err := InitWallet(wc, log, true)
	if err != nil {
		t.Fatal("Failed to setup wallet")
	}

	dc := InitDIDDummy("12512hdjdfjutigkglvmjgutyt78ddhfj")
	cs := CreditSignature{
		Signature:     "129120120221012sksfjfjfff;f",
		PrivSingature: "shsdhsdksksdisdjsdjds",
		DID:           "sksdjdsusdusd7ssdhshsjsdidsisduyrrh",
	}
	jb, err := json.Marshal(cs)
	if err != nil {
		t.Fatal("Failed to parse json")
	}
	qs := make([]string, 0)
	qs = append(qs, string(jb))
	qs = append(qs, string(jb))
	qs = append(qs, string(jb))
	qs = append(qs, string(jb))
	qs = append(qs, string(jb))

	tcb := TokenChainBlock{
		TransactionType: TokenGeneratedType,
		TokenOwner:      "sjdkskdisuslsflsflf",
		Comment:         "TOken generated",
	}
	token := "17128211910102"
	ctcb := make(map[string]interface{})
	ctcb[token] = nil

	ftcb := CreateTCBlock(ctcb, &tcb)

	s, err := GetTCHash(ftcb)
	if err != nil {
		t.Fatal("Failed to get hash")
	}
	sig, err := dc.PvtSign([]byte(s))
	if err != nil {
		t.Fatal("Failed to get signature")
	}
	ftcb[TCSignatureKey] = util.HexToStr(sig)

	blk, err := TCBEncode(ftcb)
	if err != nil {
		t.Fatal("Invalid block")
	}

	err = w.AddTokenBlock(token, blk)
	if err != nil {
		t.Fatal("Failed to add latest token chain block")
	}

	pm := make([]string, 0)
	pm = append(pm, "29202088djdhfyf76g sjskd9sd8sd7sdhfjfk")
	tcb = TokenChainBlock{
		TransactionType:   TokenTransferredType,
		TokenOwner:        "12512hdjdfjutigkglvmjgutyt78ddhfj",
		SenderDID:         "sjdkskdisuslsflsflf",
		ReceiverDID:       "12512hdjdfjutigkglvmjgutyt78ddhfj",
		Comment:           "Dummy transfer",
		TID:               "19120djdjf88ddkdfkdflddf0dodkdf",
		WholeTokens:       []string{"29202088djdhfyf76g"},
		WholeTokensID:     []string{"17281920201019128"},
		QuorumSignature:   qs,
		TokensPledgedFor:  []string{"29202088djdhfyf76g"},
		TokensPledgedWith: []string{"sjskd9sd8sd7sdhfjfk"},
		TokensPledgeMap:   pm,
	}

	rtcb, err := w.GetLatestTokenBlock(token)
	if err != nil {
		t.Fatal("Failed to read latest token chain block")
	}
	ctcb = make(map[string]interface{})
	ctcb[token] = rtcb
	stcb := CreateTCBlock(ctcb, &tcb)

	s, err = GetTCHash(stcb)
	if err != nil {
		t.Fatal("Failed to get hash")
	}
	sig, err = dc.PvtSign([]byte(s))
	if err != nil {
		t.Fatal("Failed to get signature")
	}
	stcb[TCSignatureKey] = util.HexToStr(sig)

	blk, err = TCBEncode(stcb)
	if err != nil {
		t.Fatal("Invalid block")
	}

	err = w.AddTokenBlock(token, blk)
	if err != nil {
		t.Fatal("Failed to add latest token chain block")
	}

	rblk, err := w.GetLatestTokenBlock(token)
	if err != nil {
		t.Fatal("Failed to read latest token chain block")
	}

	rstcb, err := TCBDecode(rblk)
	if err != nil {
		t.Fatal("Failed to read latest token chain block")
	}

	h, s, err := GetTCHashSig(rstcb)
	if err != nil {
		t.Fatal("Failed to get hash")
	}
	ok, err := dc.PvtVerify([]byte(h), util.StrToHex(s))
	if err != nil {
		t.Fatal("Failed to verify signature")
	}
	if !ok {
		t.Fatal("Failed to verify signature")
	}
	w.s.Close()
	w.tcs.Close()
	os.Remove("wallet.db")
	os.RemoveAll("tokenchainstorage")
}
