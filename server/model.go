package server

const (
	InvalidRequestErr string = "Invalid Request"
)

const (
	ReponseMsgHdr string = "response"
)

type LoginRequest struct {
	UserName string `json:"user_Name"`
	Password string `json:"password"`
}
