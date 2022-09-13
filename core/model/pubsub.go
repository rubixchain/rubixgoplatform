package model

type ExploreModel struct {
	Cmd           string   `json:"cmd"`
	PeerID        string   `json:"peer_id"`
	Status        string   `josn:"status"`
	DIDList       []string `json:"did_list"`
	TransactionID string   `json:"tid"`
	Message       string   `json:"message"`
}
