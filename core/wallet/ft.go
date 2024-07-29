package wallet

type FT struct {
	TokenID       string `gorm:"column:token_id;primaryKey"`
	FTName        string `gorm:"column:ft_name"`
	ParentTokenID string `gorm:"column:parent_token_id"`
}
