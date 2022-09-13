package model

type PeerStatusResponse struct {
	Version   string `json:"version"`
	DIDExists bool   `json:"did_exists"`
}
