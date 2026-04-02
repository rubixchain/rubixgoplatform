package models

type ConsensusRequest struct {
	ReferenceId string        `json:"referenceId"`
	Transaction *Transactions `json:"transaction"`
}

type ConsensusResponse struct {
	ReferenceId     string `json:"referenceId"`
	QuorumSignature string `json:"quorumSignature"`
	Message         string `json:"message"`
	Status          bool   `json:"status"`
}
