package wallet

type TokenStateProviderMap struct {
	TokenStateId   string `gorm:"column:token_state_id;primaryKey"`
	TokenStateData string `gorm:"column:token_state_data"`
	TokenId        string `gorm:"column:token_id"`
	DID            string `gorm:"column:did"`
	FuncID         int    `gorm:"column:func_id"`
	Role           int    `gorm:"column:role"`
}
