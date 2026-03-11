package models

type ConsensusRequest struct {
	ReferenceId        string          `json:"referenceId"`
	TransactionInfo    TransactionInfo `json:"transactioInfo"`
	InitiatorSignature string          `json:"initiatorSignature"`
}

type ConsensusResponse struct {
	ReferenceId     string `json:"referenceId"`
	QuorumSignature string `json:"quorumSignature"`
	Message         string `json:"message"`
	Status          bool   `json:"status"`
}
