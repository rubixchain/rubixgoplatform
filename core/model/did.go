package model

type GetDIDAccess struct {
	DID       string `json:"did"`
	Password  string `json:"password"`
	Token     string `json:"token"`
	Signature []byte `json:"signature"`
}

type DIDAccessResponse struct {
	BasicResponse
	Token string `json:"token"`
}

// GetDIDResponse used for get DID response
type GetDIDResponse struct {
	Status  bool     `json:"status"`
	Message string   `json:"message"`
	Result  []string `json:"result"`
}

// BasicResponse will be basic response model
type DIDResult struct {
	DID    string `json:"did"`
	PeerID string `json:"peer_id"`
}

// BasicResponse will be basic response model
type DIDResponse struct {
	Status  bool      `json:"status"`
	Message string    `json:"message"`
	Result  DIDResult `json:"result"`
}
