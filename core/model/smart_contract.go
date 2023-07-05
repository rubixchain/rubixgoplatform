package model

type NewContractEvent struct {
	Contract          string `json:"contract"`
	Did               string `json:"did"`
	ContractBlockHash string `json:"contract_block_hash"`
}
type NewSubscription struct {
	Contract string `json:"contract"`
}
