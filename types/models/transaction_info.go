package models

import (
	"encoding/json"
	"time"
)

// TxQueryFilter holds optional filter parameters for GetTransactionsByDIDAndTokenType.
// Zero values mean "no filter": Latest=0 means no limit, zero Time means no date bound,
// nil epoch pointers mean no epoch bound.
type TxQueryFilter struct {
	Latest     int        // if > 0, return only the N most recent transactions sorted by epoch desc
	StartDate  time.Time  // if non-zero, include only transactions with created_at >= StartDate
	EndDate    time.Time  // if non-zero, include only transactions with created_at <= EndDate
	StartEpoch *int       // if non-nil, include only transactions with epoch >= *StartEpoch
	EndEpoch   *int       // if non-nil, include only transactions with epoch <= *EndEpoch
}

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
	// Properties must stay last and keep omitempty: without it every
	// transaction emits "properties":null and hashes differently from legacy
	// nodes. Pinned by transaction_info_hash_test.go.
	Properties []*TokenInfo `json:"properties,omitempty"`
}

type TokenInfo struct {
	TokenID               string  `json:"tokenId"`
	PreviousTransactionID string  `json:"previousTransactionID"`
	Data                  string  `json:"data"`
	TokenValue            float64 `json:"tokenValue"`
	//DID                   string  `json:"did"`
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
	RBT                  float64             `json:"rbt"`
	FT                   []FTInfo            `json:"ft"`
	NFT                  []NFTInfo           `json:"nft"`
	SmartContract        []SmartContractInfo `json:"smartContract"`
	TransferNFTOwnership bool                `json:"transferNftOwnership"`
	// BurnNFT, when true, permanently destroys the NFTs listed in NFT rather
	// than executing or transferring them. A burn is a non-consensus,
	// self-signed transaction: it is persisted locally and published to both
	// the rubix_txn stream (so quorums that pledged against this NFT can
	// release their collateral) and the NFT's own topic (so subscribers learn
	// the NFT is dead). Mutually exclusive with TransferNFTOwnership, and
	// cannot be combined with RBT/FT/SmartContract in the same request.
	BurnNFT bool `json:"burnNft"`
	// SetProperties, when true, creates or edits the properties token governing
	// the NFTs listed in NFT. Only the deployer may edit an existing one.
	SetProperties bool `json:"setProperties"`
	// Properties is the document to write when SetProperties is true.
	Properties *PropertiesInfo `json:"properties,omitempty"`
}

// PropertiesInfo is the request shape for a properties document. It is a
// request type, not part of the signed payload, so field order is not
// hash-critical.
type PropertiesInfo struct {
	Transferable          bool     `json:"transferable"`
	ValidFrom             int64    `json:"validFrom,omitempty"`
	ValidTo               int64    `json:"validTo,omitempty"`
	Whitelist             []string `json:"whitelist,omitempty"`
	Admins                []string `json:"admins,omitempty"`
	AllowedSubnets        []string `json:"allowedSubnets,omitempty"`
	AllowedSmartContracts []string `json:"allowedSmartContracts,omitempty"`
}

// ToDocument converts the request shape into the stored document. Whitelist and
// Admins become CIDs once uploaded, so they are filled in by the caller.
func (p *PropertiesInfo) ToDocument() *TokenProperties {
	doc := &TokenProperties{
		Version: PropertiesDocVersion,
		Policy:  PropertiesPolicy{ValidFrom: p.ValidFrom, ValidTo: p.ValidTo},
		Restriction: PropertiesRestrict{
			AllowedSubnets:        p.AllowedSubnets,
			AllowedSmartContracts: p.AllowedSmartContracts,
		},
	}
	if p.Transferable {
		doc.Flags |= FlagTransferable
	}
	return doc
}

// IsPropertiesSet reports whether this request carries a properties write.
func (tr *TransactionRequest) IsPropertiesSet() bool {
	return tr.Tokens.SetProperties
}

// Gave the key names as FTInfo, NFTInfo and SmartContract info for now as these are the informations which are required to perform the transfer operation
type FTInfo struct {
	FTName      string  `json:"ftName"`
	NumberOfFts float64 `json:"numberOfFts"`
	CreatorDID  string  `json:"creatorDID"`
}

type NFTInfo struct {
	NFTId string  `json:"nftId"`
	Value float64 `json:"value"`
	Data  string  `json:"data"`
	// ParentNFTId, when non-empty, signals a child-mint instruction: this entry
	// mints a brand-new child NFT linked to the named parent. The parent is
	// executed in the same transaction. NFTId, when set, is used as the child's
	// ID (must be CID-shaped and not already exist locally); leave it empty to
	// have the server derive one via IPFS-add of parentNFTId+uuid.
	ParentNFTId string `json:"parentNFTId,omitempty"`
}

type SmartContractInfo struct {
	SmartContractId string  `json:"smartContractId"`
	Value           float64 `json:"value"`
	Data            string  `json:"data"`
}

type PledgeTokenRequest struct {
	ReferenceId       string  `json:"referenceId"`
	TransactionValue  float64 `json:"transactionValue"`
	InitiatorPeerInfo *DID    `json:"initiator_info"`
}

type PledgeTokenResponse struct {
	ReferenceId  string       `json:"referenceId"`
	PledgeTokens []*TokenInfo `json:"pledgeTokens"`
}

type SendTokensRequest struct {
	Tokens               *TransactionTokens `json:"tokens"`
	NFTOwnershipTransfer bool               `json:"nftOwnershipTransfer"`
	TransactionInfo      *TransactionInfo   `json:"transactionInfo"`
	Signature            *Signature         `json:"signature"`
	InitiatorPeerInfo    *DID               `json:"initiator_info"`
}

// TokenChainResponse represents a single transaction in the smart contract token chain
type TokenChainResponse struct {
	TransactionID string `json:"transactionId"`
	Initiator     string `json:"initiator"`
	Epoch         int    `json:"epoch"`
	Data          string `json:"data"`
}

// SerializeTransactionInfo produces a deterministic JSON encoding of txInfo.
// TransactionInfo and all nested types (TransactionTokens, TokenInfo, QuorumInfo) use only
// struct/slice/primitive fields — no map fields — so json.Marshal output is field-order-deterministic.
// This is the single source of truth for all hashing, signing, and persistence of TransactionInfo.
// Struct field order MUST NOT change; do NOT introduce map fields into TransactionInfo or its nested types.
func SerializeTransactionInfo(txInfo *TransactionInfo) ([]byte, error) {
	return json.Marshal(txInfo)
}
