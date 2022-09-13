package server

import "github.com/rubixchain/rubixgoplatform/core/model"

const (
	InvalidRequestErr string = "Invalid Request"
)

const (
	DIDConfigField string = "did_config"
)

const (
	ReponseMsgHdr string = "response"
)

type Request struct {
}

// Response used as model for the API responses
type Response struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result"`
}

// BootStrapResponse used as model for the API responses
type BootStrapResponse struct {
	Status  bool                 `json:"status"`
	Message string               `json:"message"`
	Result  model.BootStrapPeers `json:"result"`
}

type DIDResponse struct {
	DID string `json:"did"`
}
