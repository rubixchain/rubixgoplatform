package parts

import (
	// "context"
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
	// Pin(hash string) error
	// Request(command string, args ...string) *ipfsnode.RequestBuilder
	// SwarmConnect(ctx context.Context, addr string) error
	// Unpin(hash string) error
	// executeWithMetrics(ctx context.Context, operationName string, metadata map[string]interface{}, operation func() error) error
}

func createChildTokenContent(parentTokenContent string, index int) string {
	return parentTokenContent + "-" + fmt.Sprint(index)
}

// TODO: Current implementation references (c *Core).createPartToken
// which uses RAC for construction of child token. In future, the implementation
// will undergo change once its decided to move away from RAC.
func createChildToken(
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
		return "", nil
	}

	if len(racBlock) != 1 {
		return "", fmt.Errorf("failed to create RAC genesis block for parentToken: %v and index: %v", parentTokenID, index)
	}

	childTokenContent := createChildTokenContent(parentTokenContent, index)
	childTokenContentBuffer := bytes.NewBufferString(childTokenContent)
	childTokenHash, err := ipfsOps.Add(childTokenContentBuffer)
	if err != nil {
		return "", nil
	}

	
	return childTokenHash, nil
}

func burnParentToken(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string, parentTokenValue float64, partTokenIDs []string,  did string, isTestnet bool ) error {
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
		return fmt.Errorf("failed to create new burnt block for parent token: %v", parentTokenID) 
	}

	err	:= burntParentTokenBlock.UpdateSignature(dc)
	if err != nil {
		return fmt.Errorf("failed to sign the burnt block for parent token, err: %v", err)
	}

	err = w.AddTokenBlock(parentTokenID, burntParentTokenBlock)
	if err != nil {
		return fmt.Errorf("failed to token block, err: %v", err)
	}

	parentTokenInfo, err := w.GetToken(parentTokenID, wallet.TokenIsLocked)
	if err != nil {
		return fmt.Errorf("unexpected error: failed to fetch parent token info from SQL, err: %v. If",
		 " error is 'no records found', it is possibly due to the token having invalid token_status as it",
		 " is expected to be Locked state")
	}

	parentTokenInfo.TokenStatus = wallet.TokenIsBurnt
	err = w.UpdateToken(parentTokenInfo)
	if err != nil {
		return fmt.Errorf("failed while updating Parent token %v status to burnt, err: %v", parentTokenID, err)
	}

	return nil
}

func createChildTokenForLevel(dc did.DIDCrypto, w *wallet.Wallet, parentTokenHash string, level int, ipfsOps IPFSOperation, isTestnet bool) ([]*wallet.Token, error) {
	var createdPartTokens []*wallet.Token = make([]*wallet.Token, 0)
	
	quantity := SplitFactor(level)
	childTokenValue, err := wallet.IdxToDenom(level)
	if err != nil {
		return nil, err
	}
	parentTokenValue, err := wallet.IdxToDenom(level - 1)
	if err != nil {
		return nil, err
	}

	did := dc.GetDID()

	// Get Parent Token Details
	ipfsCatInfo, err := ipfsOps.Cat(parentTokenHash)
	if err != nil {
		return nil, err
	}

	ipfsCatBytes, err := io.ReadAll(ipfsCatInfo)
	if err != nil {
		return nil, err
	}

	parentTokenContent := string(ipfsCatBytes)

	var childTokenIDs []string = make([]string, 0)
	for i := 1; i <= quantity; i++ {
		childTokenID, err := createChildToken(parentTokenHash, parentTokenContent, i, 
			ipfsOps, did, childTokenValue)
		if err != nil {
			return nil, fmt.Errorf("failed to create child token with ID: %v, err: %v", childTokenID, err)
		}

		childTokenIDs = append(childTokenIDs, childTokenID)

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
						Token:         childTokenID,
						ParentID:      parentTokenHash,
					},
				},
			},
			TokenValue: childTokenValue,
		}

		ctcb := make(map[string]*block.Block)
		ctcb[childTokenID] = nil
		childTokenBlock := block.CreateNewBlock(ctcb, tcb)
		if childTokenBlock == nil {
			return nil, fmt.Errorf("failed to create new block for child token: %v", childTokenID)
		}

		err = childTokenBlock.UpdateSignature(dc)
		if err != nil {
			return nil, err
		}

		err = w.AddTokenBlock(childTokenID, childTokenBlock)
		if err != nil {
			return nil, err
		}

		childToken := &wallet.Token{
			TokenID: childTokenID,
			ParentTokenID: parentTokenHash,
			TokenValue: childTokenValue,
			DID: did,
			TokenStatus: wallet.TokenIsFree,
		}

		err = w.CreateToken(childToken)
		if err != nil {
			return nil, fmt.Errorf("failed to add child token %v to DB, err: %v", childToken, err)
		}

		createdPartTokens = append(createdPartTokens, childToken)
	}
	
	err = burnParentToken(dc, w, parentTokenHash, parentTokenValue, childTokenIDs,
	did, isTestnet)
	if err != nil {
		return nil, fmt.Errorf("failed to burn part token: %v of did: %v, err: %v", parentTokenHash, did, err)
	}

	//TODO: should the DB entry part tokens be added after or before
	// the buring of parent token?

	return createdPartTokens, nil
}


