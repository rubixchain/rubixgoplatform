package model

// GetDIDResponse used for get DID response
type GetDIDResponse struct {
	Status  bool     `json:"status"`
	Message string   `json:"message"`
	Result  []string `json:"result"`
}
