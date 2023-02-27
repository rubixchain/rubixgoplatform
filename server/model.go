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

type DIDResponse struct {
	DID    string `json:"did"`
	PeerID string `json:"peer_id"`
}

type LoginRequest struct {
	UserName string `json:"user_Name"`
	Password string `json:"password"`
}
