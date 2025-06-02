package model

type NewContractEvent struct {
	SmartContractToken     string `json:"smartContractToken"`
	Did                    string `json:"did"`
	Type                   int    `json:"type"`
	SmartContractBlockHash string `json:"smartContractBlockHash"`
	SmartContractData      string `json:"smartContractData"`
}

type NewSubscription struct {
	SmartContractToken string `json:"smartContractToken"`
}
