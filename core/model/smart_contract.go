package model

type NewContractEvent struct {
	SmartContractToken     string `json:"smartContractToken"`
	Did                    string `json:"did"`
	Type                   int    `json:"type"`
	SmartContractData      string `json:"smartContractData"`
}

type NewSubscription struct {
	SmartContractToken string `json:"smartContractToken"`
}
