package types

type SyncTokenRecordRequest struct {
	TokenIDs []string `json:"token_ids"`
}

type SyncTokenRecordResponse struct {
	// Value represents the token record from `tokens table` for a tokenID which is the Key
	ResultMap map[string][]byte `json:"result_map"`
}