package wallet

type FT struct {
	FTName        string `gorm:"column:ft_name;primaryKey"`
	TokenID       string `gorm:"column:token_id"`
	ParentTokenID string `gorm:"column:parent_token_id"`
}
