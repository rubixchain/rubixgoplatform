package model

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
