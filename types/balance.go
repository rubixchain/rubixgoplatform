package types

type RBTBalance struct {
	RBTBalance float64 `json:"balance"`
	PledgedRBT float64 `json:"pledged"`
	LockedRBT  float64 `json:"locked"`
}

type FTBalance struct {
	FTName     string  `json:"name"`
	CreatorDID string  `json:"creator"`
	FTValue    float64 `json:"value"`
	FTCount    int     `json:"count"`
}

type NFTBalance struct {
	NFTId    string  `json:"nft_id"`
	NFTValue float64 `json:"value"`
}

type DIDBalances struct {
	DID        string       `json:"did"`
	RBTBalance RBTBalance   `json:"rbt_balance"`
	FTBalance  []FTBalance  `json:"ft_balance"`
	NFTBalance []NFTBalance `json:"nft_balance"`
}
