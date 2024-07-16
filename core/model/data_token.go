package model

type DataToken struct {
	TokenID      string `gorm:"column:token_id;primaryKey" json:"token_id"`
	DID          string `gorm:"column:did" json:"did"`
	CommitterDID string `gorm:"column:commiter_did" json:"comiter_did"`
	BatchID      string `gorm:"column:batch_id" json:"batch_id"`
	TokenStatus  int    `gorm:"column:token_status;" json:"token_status"`
}

type DataTokenResponse struct {
	BasicResponse
	Tokens []DataToken `json:"tokens"`
}
