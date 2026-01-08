package parts

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
)


func splitAtLevel(dc did.DIDCrypto, w *wallet.Wallet, did string, tokenList []*wallet.Token, balanceDenom []string, level int, ipfsOps IPFSOperation, isTestnet bool) ([]*wallet.Token, error) {
	fmt.Printf("Step 1: Inside splitAtLevel: token denom arr: %v", balanceDenom)
	balanceDenomCount, err := wallet.GetTokenDenomCount(balanceDenom, level)
	if err != nil {
		return nil, err
	}

	if balanceDenomCount <= 0 {
		return nil, fmt.Errorf("splitOnce: invalid split attempt")
	}

	errDecrementBalanceDenom := wallet.DecrementTokenDenomArrayAtIndex(balanceDenom, level, 1)
	if errDecrementBalanceDenom != nil {
		return nil, fmt.Errorf("splitOnce: %v", err)
	}

	denomAtLevel, err := wallet.IdxToDenom(level)
	if err != nil {
		return nil, err
	}	

	parentToken, err := w.GetTokenByValueAndStatus(did, denomAtLevel, wallet.TokenIsFree)
	if err != nil {
		return nil, fmt.Errorf("splitAtLevel: error occurred while looking for an available parent token in DB for denom:%v, err: %v", denomAtLevel, err)
	}

	// Splitting of Tokenns
	childLevel := level + 1
	factor := SplitFactor(childLevel)

	errIncrement := wallet.IncrementTokenDenomArrayAtIndex(balanceDenom, childLevel, factor)
	if errIncrement != nil {
		return nil, fmt.Errorf("splitOnce increment, err: %v", errIncrement)
	}

	parentTokenID := parentToken.TokenID
	_, err = createChildTokenForLevel(dc, w, parentTokenID, childLevel, ipfsOps, isTestnet)
	if err != nil {
		return nil, fmt.Errorf("splitAtLevel: failed to create child tokens for Parent token: %v, err: %v", parentTokenID, err)
	}

	errUpdateTokenDenom := w.UpdateTokenDenomRaw(balanceDenom, did)
	if errUpdateTokenDenom != nil {
		return nil, fmt.Errorf("unable to update token denom after spliting of parent token: %v, err: %v", parentTokenID, errUpdateTokenDenom)
	}

	tokenList = removeTokenFromList(tokenList, parentTokenID)

	fmt.Printf("Step n: Inside splitAtLevel: token denom arr: %v", balanceDenom)
	return tokenList, nil
}

func removeTokenFromList(tokenList []*wallet.Token, tokenID string) []*wallet.Token {
	for idx, tkn := range tokenList {
		fmt.Printf("Token found in list: %v", tkn.TokenID)
		if tkn.TokenID == tokenID {
			fmt.Printf("Found the mathch: %v -- %v", tkn.TokenID, tokenID)
			return append(tokenList[:idx], tokenList[idx+1:]...)
		}
	}

	return tokenList
}

func SplitFactor(level int) int {
	if level%2 == 1 {
		return 2
	}

	return 5
}

