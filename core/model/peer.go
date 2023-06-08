package model

type PeerStatusResponse struct {
	Version   string `json:"version"`
	DIDExists bool   `json:"did_exists"`
}

type PeerTokenCountResponse struct {
	DIDBalance float64 `json:"balance"`
}
