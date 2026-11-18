package types

import "encoding/json"

type RecoveryRequest struct {
	DID       string `json:"did"`
	Network   string `json:"network"`
	Purpose   string `json:"purpose"`
	Epoch     int64  `json:"epoch"` // Unix time in seconds when the request is generated.
	Signature string `json:"signature"`
}

// SigningBytes returns the request fields without the signature.
// Keep this field order and JSON encoding the same on every node.
func (r RecoveryRequest) SigningBytes() ([]byte, error) {
	return json.Marshal(struct {
		DID     string `json:"did"`
		Network string `json:"network"`
		Purpose string `json:"purpose"`
		Epoch   int64  `json:"epoch"`
	}{
		DID:     r.DID,
		Network: r.Network,
		Purpose: r.Purpose,
		Epoch:   r.Epoch,
	})
}
