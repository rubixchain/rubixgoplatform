package model

type FullNodeGenesisBlock struct {
	Token              string        `json:"token"`
	TokenType          string        `json:"tokenType"`
	TokenLevel         int           `json:"tokenLevel"`
	TokenNumber        int           `json:"tokenNumber"`
	ParentID           string        `json:"parentID"`
	GrandParentID      []string      `json:"grandParentID"`
	CommitedTokens     []TransTokens `json:"commitedTokens"`
	SmartContractValue float64       `json:"smartContractValue"`
	NFTValue           float64       `json:"nftValue"`
	NFTData            string        `json:"nftData"`
}

type TransTokens struct {
	Token       string `json:"token"`
	TokenType   int    `json:"tokenType"`
	UnplededID  string `json:"unpledgedID"`
	CommitedDID string `json:"commitedDID"`
}

type FullNodeTokenChainBlock struct {
	TransactionType    string              `json:"transactionType"`
	TokenOwner         string              `json:"owner"`
	TransInfo          *TransInfo          `json:"transInfo"`
	PledgeDetails      []PledgeDetail      `json:"pledgeDetails"`
	QuorumSignature    []CreditSignature   `json:"quorumSignature"`
	SmartContract      []byte              `json:"smartContract"`
	SmartContractData  string              `json:"smartContractData"`
	TokenValue         float64             `json:"tokenValue"`
	ChildTokens        []string            `json:"childTokens"`
	InitiatorSignature *InitiatorSignature `json:"initiatorSignature"`
	NFT                []byte              `json:"nft"`
	NFTData            string              `json:"nftData"`
	Epoch              int                 `json:"epoch"`
}

type TransInfo struct {
	SenderDID      string        `json:"senderDID"`
	ReceiverDID    string        `json:"receiverDID"`
	Comment        string        `json:"comment"`
	TID            string        `json:"tid"`
	Block          []byte        `json:"block"`
	RefID          string        `json:"refID"`
	Tokens         []TransTokens `json:"tokens"`
	DeployerDID    string        `json:"deployerDID"`
	ExecutorDID    string        `json:"executorDID"`
	PinningNodeDID string        `json:"pinningNodeDID"`
}

type PledgeDetail struct {
	Token        string `json:"token"`
	TokenType    int    `json:"tokenType"`
	DID          string `json:"did"`
	TokenBlockID string `json:"tokenBlockID"`
}

type CreditSignature struct {
	Signature     string `json:"signature"`
	PrivSignature string `json:"priv_signature"`
	DID           string `json:"did"`
	Hash          string `json:"hash"`
	SignType      string `json:"sign_type"`
}

type InitiatorSignature struct {
	NLSSShare   string `json:"nlss_share_signature"`
	PrivateSign string `json:"priv_signature"`
	DID         string `json:"InitiatorDID"`
	Hash        string `json:"hash"`
	SignType    int    `json:"sign_type"`
}

// TokenDetailsFromGenesis contains essential information about a token for explorer notification.
type TokenDetailsFromGenesis struct {
	TokenID    string  `json:"token_id"`
	TokenType  int     `json:"token_type"`
	TokenValue float64 `json:"token_value"`
}

// NotifyExplorer is the structure used to send transaction information to the explorer.
// It combines fields from PubSubTxnInfo with detailed token information.
type NotifyExplorer struct {
	BlockHash         string                    `json:"block_hash"`
	TransactionID     string                    `json:"transaction_id"`
	TxnType           string                    `json:"transaction_type"`
	AssetType         int                       `json:"asset_type"`
	FTName            string                    `json:"ft_name"`
	CreatorDID        string                    `json:"creator_did"`
	PublisherDID      string                    `json:"publisher_did"`
	ReceiverDID       string                    `json:"receiver_did"`
	TxnBlock          []byte                    `json:"block"`
	LatestBlockHeight uint64                    `json:"block_height"`
	TransactionValue  float64                   `json:"transaction_value"`
	TokenValue        float64                   `json:"token_value"`
	BlockMap          map[string]interface{}    `json:"block_map"`
	TokenDetails      []TokenDetailsFromGenesis `json:"token_Details"`
}
