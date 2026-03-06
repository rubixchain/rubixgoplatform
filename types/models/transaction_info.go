package models

type TransactionInfo struct {
	Initiator       string                `json:"initiator"`
	Owner           string                `json:"owner"`
	Epoch           int                   `json:"epoch"`
	Network         string                `json:"network"`
	Tokens          []string              `json:"tokens"`
	CommittedTokens []string              `json:"committedTokens"`
	Quorums         map[string][]string   `json:"quorums"`
	Memo            string                `json:"memo,omitempty"`
	Data            string                `json:"data,omitempty"`
}

type QuorumSignature struct {
	Did string `json:"did"`
	Signature string `json:"signature"`
}

type Signature struct {
	InitiatorSignature string `json:"initiatorSignature"`
	Quorums []QuorumSignature `json:"quorums"`
}