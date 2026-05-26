package models

type ConsensusRequest struct {
	ReferenceId        string           `json:"referenceId"`
	TransactionInfo    *TransactionInfo `json:"transactionInfo"`
	InitiatorSignature string           `json:"initiatorSignature"`
	// TransferNFTOwnership rides on the envelope (not in signed TransactionInfo)
	// so the canonical payload shape stays unchanged. Quorum uses it to gate
	// ValidateNFTTransferAuthorization.
	TransferNFTOwnership bool `json:"transferNftOwnership,omitempty"`
}

type ConsensusResponse struct {
	ReferenceId     string `json:"referenceId"`
	QuorumSignature string `json:"quorumSignature"`
	Message         string `json:"message"`
	Status          bool   `json:"status"`
}
