package core

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	tkn "github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/strutil"
)

// tokenStatusBasedOnTestnetStatus returns whole or part token id based
// on testnet flag
func tokenStatusBasedOnTestnetStatus(token *wallet.Token, isTestnet bool) int {
	var tokenType int
	if token.TokenValue == 1.0 {
		tokenType = tkn.RBTTokenType
		if isTestnet {
			tokenType = tkn.TestTokenType
		}
	} else {
		tokenType = tkn.PartTokenType
		if isTestnet {
			tokenType = tkn.TestPartTokenType
		}
	}

	return tokenType
}

// filterTokenList takes in quorum tokens and TokensTable tokens, filters and returns in the following categories:
//
// - quorumTokensUnique: These are tokens, fetched from quorum, which are NOT present in the TokensTable.
//
// - tokensTableUnique: These are tokens from TokensTable, which are NOT present in the tokens list fetched from quorum.
//
// - quorumTokensPresentInTokensTable: These are tokens, fetched from quorum, which are present in the TokensTable.
func filterTokenList(quorumTokens []wallet.Token, tokensTableTokens []wallet.Token) ([]wallet.Token, []wallet.Token, []wallet.Token) {
	quorumTokensUnique := make([]wallet.Token, 0)
	tokensTableUnique := make([]wallet.Token, 0)
	quorumTokensPresentInTokensTable := make([]wallet.Token, 0)

	tknTableMap := make(map[string]wallet.Token)
	quorumMap := make(map[string]wallet.Token)

	for _, token := range tokensTableTokens {
		tknTableMap[token.TokenID] = token
	}

	for _, token := range quorumTokens {
		quorumMap[token.TokenID] = token
	}

	for _, token := range quorumTokens {
		_, exists := tknTableMap[token.TokenID]
		if !exists {
			quorumTokensUnique = append(quorumTokensUnique, token)
		} else {
			quorumTokensPresentInTokensTable = append(quorumTokensPresentInTokensTable, token)
		}
	}

	for _, token := range tokensTableTokens {
		_, exists := quorumMap[token.TokenID]
		if !exists {
			tokensTableUnique = append(tokensTableUnique, token)
		}
	}

	return quorumTokensUnique, tokensTableUnique, quorumTokensPresentInTokensTable
}

func (c *Core) NodeSync(nodeSyncRequest *model.NodeSyncRequest) *model.NodeSyncResponse {
	nodeSyncResponse := &model.NodeSyncResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	c.log.Info("Node sync for DID %v has started")
	// TODO(Arnab): Current assumption is that zeroth index of the node will
	// be selected, as only this node will have the balance and rest won't
	// It's also because pledging quorum only has the the latest information,
	// while the rest of quorums do not.
	quorumList := c.qm.GetQuorum(2, "")
	quorumPeerAddress := quorumList[0]

	quorumPeer, err := c.getPeer(quorumPeerAddress)
	if err != nil {
		errMsg := fmt.Sprintf("Receiver not connected, err: %v", err.Error())
		nodeSyncResponse.Message = errMsg
		c.log.Error(errMsg)
		return nodeSyncResponse
	}
	defer quorumPeer.Close()

	// We are fetching a list of RBT tokens which are owned by the DID
	var getTokensByDIDResponse *model.GetTokensByDIDResponse
	jsonRequestErr := quorumPeer.SendJSONRequest("POST", APIGetTokensByDID, nil, nodeSyncRequest, &getTokensByDIDResponse, true)
	if jsonRequestErr != nil {
		errMsg := fmt.Sprintf("unable to send request, err: %v", jsonRequestErr)
		c.log.Error(errMsg)
		nodeSyncResponse.Message = errMsg
		return nodeSyncResponse
	}
	if !getTokensByDIDResponse.Status {
		errMsg := fmt.Sprintf("unable to get tokens owned by %v from quorum %v, err: %v", nodeSyncRequest.Did, quorumPeerAddress, nodeSyncResponse.Message)
		c.log.Error(errMsg)
		return nodeSyncResponse
	}

	tokensFromQuorum := getTokensByDIDResponse.Tokens
	tokensFromQuorumCopy := make([]wallet.Token, len(tokensFromQuorum))
	for k, v := range tokensFromQuorum {
		tokensFromQuorumCopy[k] = *v
	}

	var tokensFromTokensTable []wallet.Token = make([]wallet.Token, 0)
	tokensFromTokensTable, err = c.w.GetAllTokens(nodeSyncRequest.Did)
	if err != nil && !strings.Contains(err.Error(), "no records found") {
		errMsg := fmt.Sprintf("unable to fetch tokens from TokensTable of DID: %v, err: %v", nodeSyncRequest.Did, err)
		nodeSyncResponse.Message = errMsg
		c.log.Error(errMsg)
		return nodeSyncResponse
	}

	tokensFromQuorumNotPresentInTokensTable, tokensFromTokensTableNotPresentWithQuorums, quorumTokensPresentInTokensTable := filterTokenList(tokensFromQuorumCopy, tokensFromTokensTable)

	// Process TokensTable tokens which are not present in fetched quorum list
	for _, token := range tokensFromTokensTableNotPresentWithQuorums {
		tokenType := tokenStatusBasedOnTestnetStatus(&token, c.testNet)

		switch token.TokenStatus {
		case wallet.TokenIsFree:
			err := c.syncTokenChainFrom(quorumPeer, "", token.TokenID, tokenType)
			// Proceed with TokenTable update if the token chain sync is successful
			if err == nil {
				c.log.Info(fmt.Sprintf("Token chain syncing of %v has been successful. Proceeding to change the status to Transferred", token.TokenID))
				t := token
				t.TokenStatus = wallet.TokenIsTransferred
				err = c.w.UpdateToken(&t)
				if err != nil {
					errMsg := fmt.Sprintf("unable to update whole token info, err: %v", err)
					nodeSyncResponse.Message = errMsg
					c.log.Error(errMsg)
					return nodeSyncResponse
				}
			} else {
				// In case of part token and generated test whole RBT token, it could either have been transferred or
				// it could be with the owner. This information is not straightforward as generated token's information
				// is not with the quorums. Hence 
				if strings.Contains(err.Error(), "Token chain block does not exist") {
					c.log.Info(fmt.Sprintf("unable to fetch token chain for token: %v, proceeding to check if local peer is the provider", token.TokenID))
					peers, err := c.GetDHTddrs(token.TokenID)
					if err != nil {
						errMsg := fmt.Sprintf("error occurred while fetching providers for token %v, err: %v", token.TokenID, err)
						nodeSyncResponse.Message = errMsg
						c.log.Error(errMsg)
						return nodeSyncResponse
					}

					localPeerFound := false
					for _, peer := range peers {
						// Check if local peer is a provider of the token
						// No need for token status change from current Free state
						if peer == c.peerID {
							c.log.Info(fmt.Sprintf("local peer is the provider for the token: %v, skipping any status change and token chain sync for it", token.TokenID))
							localPeerFound = true
							break
						}
					}

					if !localPeerFound {
						c.log.Info(fmt.Sprintf("local peer not found in provider list for token: %v, setting its status to Transferred and skipping token chain sync", token.TokenID))
						t := token
						t.TokenStatus = wallet.TokenIsTransferred
						err = c.w.UpdateToken(&t)
						if err != nil {
							errMsg := fmt.Sprintf("unable to update token info of %v, err: %v", t.TokenID, err)
							nodeSyncResponse.Message = errMsg
							c.log.Error(errMsg)
							return nodeSyncResponse
						}
					}
				} else {
					errMsg := fmt.Sprintf("unable to sync token chain for token %v, err: %v", token.TokenID, err)
					nodeSyncResponse.Message = errMsg
					c.log.Error(errMsg)
					return nodeSyncResponse
				}
			}
		}
	}

	// Process tokens fetched from quorums which are not present in TokensTable
	for _, token := range tokensFromQuorumNotPresentInTokensTable {
		tokenType := tokenStatusBasedOnTestnetStatus(&token, c.testNet)

		err := c.syncTokenChainFrom(quorumPeer, "", token.TokenID, tokenType)
		if err != nil {
			errMsg := fmt.Sprintf("unable to sync token chain, err: %v", err)
			nodeSyncResponse.Message = errMsg
			c.log.Error(errMsg)
			return nodeSyncResponse
		}

		err = c.w.AddToken(&token)
		if err != nil {
			errMsg := fmt.Sprintf("unable to add token info, err: %v", err)
			nodeSyncResponse.Message = errMsg
			c.log.Error(errMsg)
			return nodeSyncResponse
		}
	}

	// Process tokens fetched from quorum which are present in TokensTable
	for _, token := range quorumTokensPresentInTokensTable {
		tokenType := tokenStatusBasedOnTestnetStatus(&token, c.testNet)

		err = c.syncTokenChainFrom(quorumPeer, "", token.TokenID, tokenType)
		if err != nil {
			errMsg := fmt.Sprintf("unable to sync token chain for token, err: %v", err)
			nodeSyncResponse.Message = errMsg
			c.log.Error(errMsg)
			return nodeSyncResponse
		}

		tknFromTokenTable, err := c.w.ReadToken(token.TokenID)
		if err != nil {
			errMsg := fmt.Sprintf("unable to read token info, err: %v", err)
			nodeSyncResponse.Message = errMsg
			c.log.Error(errMsg)
			return nodeSyncResponse
		}

		// Only update if there is a difference in Token Statuses
		if tknFromTokenTable.TokenStatus != token.TokenStatus {
			t := token
			t.TokenStatus = wallet.TokenIsFree
			err = c.w.UpdateToken(&t)
			if err != nil {
				errMsg := fmt.Sprintf("unable to update token info, err: %v", err)
				nodeSyncResponse.Message = errMsg
				c.log.Error(errMsg)
				return nodeSyncResponse
			}
		}

	}

	nodeSyncResponse.BasicResponse.Status = true
	return nodeSyncResponse
}

func (c *Core) getTokensByDID(req *ensweb.Request) *ensweb.Result {
	response := model.GetTokensByDIDResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
		Tokens: make([]*wallet.Token, 0),
	}

	var getTokensByDIDRequest *model.GetTokensByDIDRequest
	err := c.l.ParseJSON(req, &getTokensByDIDRequest)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse json request, err: %v", err.Error())
		c.log.Error(errMsg)
		response.Message = errMsg
		return c.l.RenderJSON(req, &response, http.StatusOK)
	}
	did := getTokensByDIDRequest.Did

	c.log.Info(fmt.Sprintf("Retreiving whole and part tokens for did %v...", did))

	// var tokens []*wallet.Token
	wholeTokens, err := c.searchWholeTokens(did)
	if err != nil {
		errMsg := fmt.Sprintf("failed while searching for whole tokens: %v", err.Error())
		c.log.Error(errMsg)
		response.Message = errMsg
		return c.l.RenderJSON(req, &response, http.StatusOK)
	}
	response.Tokens = append(response.Tokens, wholeTokens...)

	partTokens, err := c.searchPartTokens(did)
	if err != nil {
		errMsg := fmt.Sprintf("failed while searching for part tokens: %v", err.Error())
		c.log.Error(errMsg)
		response.Message = errMsg
		return c.l.RenderJSON(req, &response, http.StatusOK)
	}
	response.Tokens = append(response.Tokens, partTokens...)

	response.Status = true
	return c.l.RenderJSON(req, &response, http.StatusOK)
}

// search whole tokens
func (c *Core) searchWholeTokens(did string) ([]*wallet.Token, error) {
	var tokenList []*wallet.Token

	tokenType := tkn.RBTTokenType
	if c.testNet {
		tokenType = tkn.TestTokenType
	}

	wholeTokenKeys, err := c.w.GetAllTokenKeys(tokenType)
	if err != nil {
		return []*wallet.Token{}, err
	}
	wholeTokenHashes := c.getTokenHashesFromKeys(wholeTokenKeys)
	for _, tokenHash := range wholeTokenHashes {
		blk := c.w.GetLatestTokenBlock(tokenHash, tokenType)
		if blk == nil {
			return nil, fmt.Errorf("failed to sync restored node: unable to fetch latest block")
		}

		currentOwnerOfToken := blk.GetOwner()

		if did == currentOwnerOfToken {
			var tokenStatus int
			if blk.GetTransType() == block.TokenBurntType {
				tokenStatus = wallet.TokenIsBurnt
			} else {
				tokenStatus = wallet.TokenIsFree
			}

			token := &wallet.Token{
				TokenID:     tokenHash,
				TokenValue:  1.0,
				DID:         did,
				TokenStatus: tokenStatus,
			}
			tokenList = append(tokenList, token)
		}
	}

	return tokenList, nil
}

// search part tokens
func (c *Core) searchPartTokens(did string) ([]*wallet.Token, error) {
	var tokenList []*wallet.Token

	tokenType := tkn.PartTokenType
	if c.testNet {
		tokenType = tkn.TestPartTokenType
	}
	partTokenKeys, err := c.w.GetAllTokenKeys(tokenType)
	if err != nil {
		return []*wallet.Token{}, err
	}

	partTokenHashes := c.getTokenHashesFromKeys(partTokenKeys)
	for _, tokenHash := range partTokenHashes {
		blk := c.w.GetLatestTokenBlock(tokenHash, tokenType)
		if blk == nil {
			return nil, fmt.Errorf("failed to sync restored node: unable to fetch latest block")
		}

		currentOwnerOfToken := blk.GetOwner()
		if did == currentOwnerOfToken {
			genesisBlock := c.w.GetGenesisTokenBlock(tokenHash, tokenType)
			parent, _, err := genesisBlock.GetParentDetials(tokenHash)
			if err != nil {
				return nil, fmt.Errorf("failed to sync restored node: unable to fetch parent details block")
			}

			var partTokenValue float64 = genesisBlock.GetTokenValue()

			var tokenStatus int
			if blk.GetTransType() == block.TokenBurntType {
				tokenStatus = wallet.TokenIsBurnt
			} else {
				tokenStatus = wallet.TokenIsFree
			}

			token := &wallet.Token{
				TokenID:       tokenHash,
				TokenValue:    partTokenValue,
				ParentTokenID: parent,
				DID:           did,
				TokenStatus:   tokenStatus,
			}
			tokenList = append(tokenList, token)
		}
	}

	return tokenList, nil
}

func (c *Core) getTokenHashesFromKeys(keys []string) []string {
	var tokenHashes []string

	for _, key := range keys {
		keyElems := strings.Split(key, "-")
		tokenHash := keyElems[1]
		tokenHashes = append(tokenHashes, tokenHash)
	}

	tokenHashes = strutil.RemoveDuplicates(tokenHashes, false)
	return tokenHashes
}
