package models

type TransactionInfo struct {
	Initiator       string              `json:"initiator"`
	Owner           string              `json:"owner"`
	Epoch           int                 `json:"epoch"`
	Network         string              `json:"network"`
	Tokens          *TokenInfo          `json:"tokens"`
	CommittedTokens []string            `json:"committedTokens"`
	Quorums         map[string][]string `json:"quorums"`
	Memo            string              `json:"memo"`
	Data            string              `json:"data"`
}

type TokenInfo struct {
	RBT           []*TokenInfoDetails `json:"rbt"`
	NFT           []*TokenInfoDetails `json:"nft"`
	FT            []*TokenInfoDetails `json:"ft"`
	SmartContract []*TokenInfoDetails `json:"smart_contract"`
}

type TokenInfoDetails struct {
	TokenID               string `json:"tokenId"`
	PreviousTransactionID string `json:"previousTransactionID"`
}

type QuorumSignature struct {
	Did       string `json:"did"`
	Signature string `json:"signature"`
}

type Signature struct {
	InitiatorSignature string            `json:"initiatorSignature"`
	Quorums            []QuorumSignature `json:"quorums"`
}
