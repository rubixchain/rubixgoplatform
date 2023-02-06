package model

// BootStrapPeers
type BootStrapPeers struct {
	Peers []string `json:"peers"`
}

// BootStrapResponse used as model for the API responses
type BootStrapResponse struct {
	Status  bool           `json:"status"`
	Message string         `json:"message"`
	Result  BootStrapPeers `json:"result"`
}
