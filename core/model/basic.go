package model

// BasicResponse will be basic response model
type BasicResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}

// BootStrapPeers
type BootStrapPeers struct {
	Peers []string `json:"peers"`
}
