package parts

import (
	"bytes"
	"fmt"
	"io"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
)

const PartString = "part"

type IPFSOperation interface {
	Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error)
	AddDir(path string) (string, error)
	BootstrapAdd(peers []string) ([]string, error)
	BootstrapRmAll() ([]string, error)
	Cat(hash string) (io.ReadCloser, error)
	Get(hash string, path string) error
	ID() (*ipfsnode.IdOutput, error)
}

func createChildTokenContent(parentTokenContent string, index int) string {
	return parentTokenContent + "-" + fmt.Sprint(index)
}

// TODO: Current implementation references (c *Core).createPartToken
// which uses RAC for construction of child token. In future, the implementation
// will undergo change once its decided to move away from RAC.
func createChildTokenAtIndex(
	parentTokenID string, parentTokenContent string,
	index int, ipfsOps IPFSOperation, userDID string,
	denomValue float64,
) (string, error) {
	racType := &rac.RacType{
		Type:        7, // TODO: change this
		DID:         userDID,
		TotalSupply: 1,
		TimeStamp:   time.Now().String(),
		PartInfo: &rac.RacPartInfo{
			Parent:  parentTokenID,
			PartNum: index,
			Value:   denomValue,
		},
	}

	racBlock, err := rac.CreateRac(racType)
	if err != nil {
		return "", fmt.Errorf("createChildToken: error occured while creating RAC block, err: %v", err)
	}

	if len(racBlock) != 1 {
		return "", fmt.Errorf("createChildToken: failed to create RAC genesis block for parentToken: %v and index: %v", parentTokenID, index)
	}

	childTokenContent := createChildTokenContent(parentTokenContent, index)
	childTokenContentBuffer := bytes.NewBufferString(childTokenContent)
	childTokenHash, err := ipfsOps.Add(childTokenContentBuffer)
	if err != nil {
		return "", fmt.Errorf("createChildToken: failed to perform IPFS Add operation while fetching token hash, err: %v", err)
	}

	return childTokenHash, nil
}

func burnParentToken(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string, parentTokenValue float64, partTokenIDs []string, did string, isTestnet bool) error {
	parentTokenType := GetParentTokenType(isTestnet)

	// Burn the parent token
	bti := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     parentTokenID,
				TokenType: GetParentTokenType(isTestnet),
			},
		},
		Comment: "Token burnt at : " + time.Now().String(),
	}

	parentTokenChainBlock := &block.TokenChainBlock{
		TransactionType: block.TokenBurntType,
		TokenOwner:      did,
		TransInfo:       bti,
		TokenValue:      parentTokenValue,
		ChildTokens:     partTokenIDs,
	}

	ctcb := make(map[string]*block.Block)
	ctcb[parentTokenID] = w.GetLatestTokenBlock(parentTokenID, parentTokenType)
	burntParentTokenBlock := block.CreateNewBlock(ctcb, parentTokenChainBlock)
	if burntParentTokenBlock == nil {
		return fmt.Errorf("burnParentToken: failed to create new burnt block for parent token: %v", parentTokenID)
	}

	err := burntParentTokenBlock.UpdateSignature(dc)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to sign the burnt block for parent token, err: %v", err)
	}

	err = w.AddTokenBlock(parentTokenID, burntParentTokenBlock)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to token block, err: %v", err)
	}

	parentTokenInfo, err := w.GetToken(parentTokenID, wallet.TokenIsLocked)
	if err != nil {
		return fmt.Errorf(`burnParentToken: unexpected error: failed to fetch parent token info from SQL, err: %v. If,
		error is 'no records found', it is possibly due to the token having invalid token_status as it,
		is expected to be Locked state`, err)
	}

	parentTokenInfo.TokenStatus = wallet.TokenIsBurnt
	err = w.UpdateToken(parentTokenInfo)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed while updating Parent token %v status to burnt, err: %v", parentTokenID, err)
	}

	return nil
}

func createChildTokensAtLevel(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string, level int, ipfsOps IPFSOperation, isTestnet bool) (map[int]wallet.Token, error) {
	var childTokenIndexMap map[int]wallet.Token = make(map[int]wallet.Token)

	maxTokenCount := MaxTokensAtLevel(level)
	childTokenValue, err := LevelToDenom(level)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to fetch the denom for level: %v, err: %v", level, err)
	}

	did := dc.GetDID()

	// Get Parent Token Details
	parentTokenHeirarchicalID, err := IpfsCatString(parentTokenID, ipfsOps)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to get the content for parent token: %v, err: %v", parentTokenID, err)
	}

	for i := 1; i <= maxTokenCount; i++ {
		childTokenID, err := createChildTokenAtIndex(parentTokenID, parentTokenHeirarchicalID.String(), i,
			ipfsOps, did, childTokenValue)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to create child token with ID: %v, err: %v", childTokenID, err)
		}

		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     childTokenID,
					TokenType: GetChildTokenType(isTestnet),
				},
			},
			Comment: "Part token generated at : " + time.Now().String(),
		}

		tcb := &block.TokenChainBlock{
			TransactionType: block.TokenGeneratedType,
			TokenOwner:      did,
			TransInfo:       bti,
			GenesisBlock: &block.GenesisBlock{
				Info: []block.GenesisTokenInfo{
					{
						Token:    childTokenID,
						ParentID: parentTokenID,
					},
				},
			},
			TokenValue: childTokenValue,
		}

		ctcb := make(map[string]*block.Block)
		ctcb[childTokenID] = nil
		childTokenBlock := block.CreateNewBlock(ctcb, tcb)
		if childTokenBlock == nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to create new block for child token: %v", childTokenID)
		}

		err = childTokenBlock.UpdateSignature(dc)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to update the signature of child Token: %v, err: %v", childTokenBlock, err)
		}

		err = w.AddTokenBlock(childTokenID, childTokenBlock)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add token block for child token: %v, err: %v", childTokenID, err)
		}

		childToken := wallet.Token{
			TokenID:       childTokenID,
			ParentTokenID: parentTokenID,
			TokenValue:    childTokenValue,
			DID:           did,
			TokenStatus:   wallet.TokenIsFree,
		}

		err = w.CreateToken(&childToken)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add child token %v to DB, err: %v", childToken, err)
		}

		childTokenIndexMap[i] = childToken
	}

	return childTokenIndexMap, nil
}
