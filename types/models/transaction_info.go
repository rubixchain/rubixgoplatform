package models

type TransactionInfo struct {
	Initiator       string             `json:"initiator"`
	Owner           string             `json:"owner"`
	Epoch           int                `json:"epoch"`
	Network         string             `json:"network"`
	Tokens          *TransactionTokens `json:"tokens"`
	CommittedTokens []*TokenInfo       `json:"committedTokens"`
	Quorums         []*QuorumInfo      `json:"quorums"`
	Memo            string             `json:"memo"`
}

type TransactionTokens struct {
	RBT           []*TokenInfo `json:"rbt"`
	NFT           []*TokenInfo `json:"nft"`
	FT            []*TokenInfo `json:"ft"`
	SmartContract []*TokenInfo `json:"smartContract"`
}

type TokenInfo struct {
	TokenID               string `json:"tokenId"`
	PreviousTransactionID string `json:"previousTransactionID"`
	Data                  string `json:"data"`
}

type QuorumInfo struct {
	Did    string       `json:"did"`
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

// Request to initiate transfer
type TransactionRequest struct {
	Initiator string                  `json:"initiator"`
	Owner     string                  `json:"owner"`
	Tokens    TransactionTokenDetails `json:"tokens"`
	Memo      string                  `json:"memo"`
}

// The struct which contains all the information of the tokens that are being transferred
// This struct contains all the informations to facilitate transfer of multiple types of tokens via a single request
type TransactionTokenDetails struct {
	RBT           float64             `json:"rbt"`
	FT            []FTInfo            `json:"ft"`
	NFT           []NFTInfo           `json:"nft"`
	SmartContract []SmartContractInfo `json:"smartContract"`
}

// Gave the key names as FTInfo, NFTInfo and SmartContract info for now as these are the informations which are required to perform the transfer operation
type FTInfo struct {
	FTName      string  `json:"ftName"`
	NumberOfFts float64 `json:"numberOfFts"`
	CreatorDID  string  `json:"creatorDID"`
}

type NFTInfo struct {
	NFTId             string  `json:"nftId"`
	Value             float64 `json:"value"`
	Data              string  `json:"data"`
	TransferOwnerShip bool    `json:"transferOwnerShip"`
}

type SmartContractInfo struct {
	SmartContractId string  `json:"smartContractId"`
	Value           float64 `json:"value"`
	Data            string  `json:"data"`
}

type PledgeTokenRequest struct {
	ReferenceId      string  `json:"referenceId"`
	TransactionValue float64 `json:"transactionValue"`
}

type PledgeTokenResponse struct {
	ReferenceId  string       `json:"referenceId"`
	PledgeTokens []*TokenInfo `json:"pledgeTokens"`
}

type SendTokensRequest struct {
	Tokens               *TransactionTokens `json:"tokens"`
	NFTOwnershipTransfer bool               `json:"nftOwnershipTransfer"`
}
