package server

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
type Repsonse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result"`
}

type DIDResponse struct {
	DID string `json:"did"`
}
