package types

// DIDCrypto defines the interface for DID cryptographic operations
type DIDCrypto interface {
	GetDID() string
	Sign(hash []byte) ([]byte, error)
	SignVerify(hash []byte, sign []byte) (bool, error)
}

type DIDCreate struct {
	PrivPWD   string `json:"password"`
	PubKey    string `json:"public_key"`
	PrivKey   string `json:"private_key"`
	Mnemonic  string `json:"mnemonic"`
	ChildPath int    `json:"childPath"`
}

type SignReqData struct {
	ID   string `json:"id"`
	Hash []byte `json:"hash"`
}

type SignRespData struct {
	ID        string `json:"id"`
	Password  string `json:"password"`
	Signature string `json:"signature"` // signature string should be base64
}

// BootStrapResponse used as model for the API responses
type SignResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  SignReqData `json:"result"`
}
