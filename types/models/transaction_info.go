package models

type TransactionInfo struct {
	Initiator       string                 `json:"initiator"`
	Owner           string                 `json:"owner"`
	Epoch           int                    `json:"epoch"`
	Network         string                 `json:"network"`
	Tokens          *TransactionTokens     `json:"tokens"`
	CommittedTokens []*TokenInfo            `json:"committedTokens"`
	Quorums         []*QuorumInfo `json:"quorums"`
	Memo            string                 `json:"memo"`
}

type TransactionTokens struct {
	RBT           []*TokenInfo `json:"rbt"`
	NFT           []*TokenInfo `json:"nft"`
	FT            []*TokenInfo `json:"ft"`
	SmartContract []*TokenInfo `json:"smart_contract"`
}

type TokenInfo struct {
	TokenID               string `json:"tokenId"`
	PreviousTransactionID string `json:"previousTransactionID"`
	Data string `json:"data"`
}

type QuorumInfo struct {
	Did string `json:"did"`
	Tokens []*TokenInfo `json:"tokens"`
}

type QuorumSignature struct {
	Did       string `json:"did"`
	Signature string `json:"signature"`
}

type Signature struct {
	InitiatorSignature string            `json:"initiatorSignature"`
	Quorums            []QuorumSignature `json:"quorums"`
}
