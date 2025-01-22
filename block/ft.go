package block

const (
	FTNameKey   string = "1"
	FTSymbolKey string = "2"
	FTSupplyKey string = "3"
	FTNumberKey string = "4"
)

func newFTData(ftData *FTData) map[string]interface{} {
	if ftData == nil {
		return nil
	}
	nftd := make(map[string]interface{})
	if ftData.FTName != "" {
		nftd[FTNameKey] = ftData.FTName
	}
	if ftData.FTSymbol != "" {
		nftd[FTSymbolKey] = ftData.FTSymbol
	}
	if ftData.FTCount > 0 {
		nftd[FTSupplyKey] = ftData.FTCount
	}
	if ftData.FTNum > 0 {
		nftd[FTNumberKey] = ftData.FTNum
	}
	return nftd
}
