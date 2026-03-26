package types

// DIDCrypto defines the interface for DID cryptographic operations
type DIDCrypto interface {
	GetDID() string
	GetSignType() int
	Sign(hash string) ([]byte, []byte, error)
	PvtSign(hash []byte) ([]byte, error)
	PvtVerify(hash []byte, sign []byte) (bool, error)
}

type DIDCreate struct {
	PrivPWD   string `json:"priv_pwd"`
	PubKey    string `json:"pub_key"`
	PrivKey   string `json:"priv_key"`
	Mnemonic  string `json:"mnemonic"`
	ChildPath int    `json:"childPath"`
}
