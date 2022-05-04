package server

const (
	InvalidRequestErr string = "Invalid Request"
)

const (
	ReponseMsgHdr string = "response"
)

type Request struct {
}

// Response used as model for the API responses
type Repsonse struct {
	Data    map[string]string `json:"data"`
	Message string            `jsom:"message"`
	Status  string            `jsom:"status"`
}
