package types

// DIDCrypto defines the interface for DID cryptographic operations
type DIDCrypto interface {
	GetDID() string
	GetSignType() int
	Sign(hash string) ([]byte, []byte, error)
	PvtSign(hash []byte) ([]byte, error)
	PvtVerify(hash []byte, sign []byte) (bool, error)
}
