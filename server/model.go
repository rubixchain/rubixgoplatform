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

type LoginRequest struct {
	UserName string `json:"user_Name"`
	Password string `json:"password"`
}
