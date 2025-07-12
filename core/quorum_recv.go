package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/service"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	didcrypto "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (c *Core) addUnpledgeDetails(req *ensweb.Request) *ensweb.Result {
	resp := model.BasicResponse{
		Status: false,
	}

	var serverReq *model.AddUnpledgeDetailsRequest
	c.l.ParseJSON(req, &serverReq)

	var transactionID string = serverReq.TransactionHash
	var quorumDID string = serverReq.QuorumDID
	var pledgeTokenHashes []string = serverReq.PledgeTokenHashes
	var transactionEpoch int64 = serverReq.TransactionEpoch

	if len(pledgeTokenHashes) == 0 {
		c.log.Error("unable to get information about pledge token hashes")
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

	// Add Unpledge details to UnpledgeSequence table
	pledgeTokenHashesStrArr := strings.Join(pledgeTokenHashes, ",")

	unpledgeSequenceInfo := &wallet.UnpledgeSequenceInfo{
		TransactionID: transactionID,
		PledgeTokens:  pledgeTokenHashesStrArr,
		Epoch:         transactionEpoch,
		QuorumDID:     quorumDID,
	}

	err := c.w.AddUnpledgeSequenceInfo(unpledgeSequenceInfo)
	if err != nil {
		resp.Message = fmt.Sprintf("Error while adding record to UnpledgeSequence table for txId: %v, error: %v", transactionID, err.Error())
		c.log.Error(fmt.Sprintf("Error while adding record to UnpledgeSequence table for txId: %v, error: %v", transactionID, err.Error()))
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

	resp.Status = true
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) creditStatus(req *ensweb.Request) *ensweb.Result {
	// ::TODO:: Get proper credit score
	did := c.l.GetQuerry(req, "did")
	credits, err := c.w.GetCredit(did)
	var cs model.CreditStatus
	cs.Score = 0
	if err == nil {
		cs.Score = len(credits)
	}
	return c.l.RenderJSON(req, &cs, http.StatusOK)
}

func (c *Core) verifyContract(cr *ConensusRequest, self_did string) (bool, *contract.Contract) {
	sc := contract.InitContract(cr.ContractBlock, nil)
	// setup the did to verify the signature
	dc, err := c.SetupForienDID(sc.GetSenderDID(), self_did)
	if err != nil {
		c.log.Error("Failed to get DID", "err", err)
		return false, nil
	}
	err = sc.VerifySignature(dc)
	if err != nil {
		c.log.Error("Failed to verify sender signature in verifyContract", "err", err)
		return false, nil
	}
	return true, sc
}

func (c *Core) quorumDTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	ok, sc := c.verifyContract(cr, "")
	if !ok {
		crep.Message = "Failed to verify sender signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	//check if token has multiple pins
	dt := sc.GetTransTokenInfo()
	if dt == nil {
		c.log.Error("Consensus failed, data token missing")
		crep.Message = "Consensus failed, data token missing"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	address := cr.SenderPeerID + "." + sc.GetSenderDID()
	p, err := c.getPeer(address)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		crep.Message = "Failed to get peer"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	defer p.Close()
	for k := range dt {
		err := c.syncTokenChainFrom(p, dt[k].BlockID, dt[k].Token, dt[k].TokenType)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			crep.Message = "Failed to sync token chain block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if dt[k].TokenType == token.DataTokenType {
			c.ipfs.Pin(dt[k].Token)
		}
	}
	qHash := util.CalculateHash(sc.GetBlock(), "SHA3-256")
	qsb, ppb, err := qdc.Sign(util.HexToStr(qHash))
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		crep.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	c.log.Debug("Data Consensus finished")
	crep.Status = true
	crep.Message = "Conensus finished successfully"
	crep.ShareSig = qsb
	crep.PrivSig = ppb
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) quorumRBTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}

	c.log.Debug("********** conesnsus request id : ", cr.ReqID)

	ok, sc := c.verifyContract(cr, did)
	if !ok {
		crep.Message = "failed to verify sender signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	//initiate trans token block
	transTknBlock := block.InitBlock(cr.TransTokenBlock, nil, block.NoSignature())
	if transTknBlock == nil {
		c.log.Error("Failed to do signature, invalid token chain block")
		crep.Message = "Failed to do signature, invalid token chanin block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	// //Validate sender signature
	// response, err := c.ValidateTxnInitiator(transTknBlock)
	// if err != nil {
	// 	c.log.Error("signature request failed, msg", response.Message, "err", err)
	// 	crep.Message = response.Message
	// 	return c.l.RenderJSON(req, &crep, http.StatusOK)
	// }

	//check if token has multiple pins
	ti := sc.GetTransTokenInfo()
	results := make([]MultiPinCheckRes, len(ti))
	var wg sync.WaitGroup
	var receiverPeerId = cr.ReceiverPeerID
	if receiverPeerId == "" {
		c.log.Debug("Receiver peer id is nil: checking for pinning node peer id")
		receiverPeerId = cr.PinningNodePeerID
		c.log.Debug("Pinning Node Peer Id", receiverPeerId)
	}
	for i := range ti {
		wg.Add(1)
		if cr.Mode == SpendableRBTTransferMode {
			go c.pinCheck(ti[i].Token, i, cr.ReceiverPeerID, "", results, &wg)
		} else {
			go c.pinCheck(ti[i].Token, i, cr.SenderPeerID, cr.ReceiverPeerID, results, &wg)
		}
	}
	wg.Wait()
	for i := range results {
		if results[i].Error != nil {
			c.log.Error("Error occured", "error", results[i].Error)
			crep.Message = "Error while cheking Token multiple Pins"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if results[i].Status {
			c.log.Error("Token has multiple owners", "token", results[i].Token, "owners", results[i].Owners)
			crep.Message = "Token has multiple owners"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}

	// check token ownership

	validateTokenOwnershipVar, err, _ := c.validateTokenOwnership(cr, sc, did)
	if err != nil {
		validateTokenOwnershipErrorString := fmt.Sprint(err)
		if strings.Contains(validateTokenOwnershipErrorString, "parent token is not in burnt stage") {
			crep.Message = "Token ownership check failed, err: " + validateTokenOwnershipErrorString
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if strings.Contains(validateTokenOwnershipErrorString, "failed to sync tokenchain Token") {
			crep.Message = "Token ownership check failed, err: " + validateTokenOwnershipErrorString
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		c.log.Error("Tokens ownership check failed")
		crep.Message = "Token ownership check failed, err : " + err.Error()
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	if !validateTokenOwnershipVar {
		c.log.Error("Tokens ownership check failed")
		crep.Message = "Token ownership check failed"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	/* 	if !c.validateTokenOwnership(cr, sc) {
		c.log.Error("Token ownership check failed")
		crep.Message = "Token ownership check failed"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	} */

	//Token state check and pinning
	/*
		1. get the latest block from token chain,
		2. retrive the Block Id
		3. concat token id and blockId
		4. add to ipfs
		5. check for pin and if none pin the content
		6. if pin exist , exit with error token state exhauste
	*/

	tokenStateCheckResult := make([]TokenStateCheckResult, len(ti))
	c.log.Debug("entering validation to check if token state is exhausted, ti len", len(ti))
	for i := range ti {
		wg.Add(1)
		go c.checkTokenState(ti[i].Token, did, i, tokenStateCheckResult, &wg, cr.QuorumList, ti[i].TokenType)
	}
	wg.Wait()

	for i := range tokenStateCheckResult {
		if tokenStateCheckResult[i].Error != nil {
			c.log.Error("Error occured", "error", tokenStateCheckResult[i].Error)
			crep.Message = "Error while cheking Token State Message : " + tokenStateCheckResult[i].Message
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if tokenStateCheckResult[i].Exhausted {
			c.log.Debug("Token state has been exhausted, Token being Double spent:", tokenStateCheckResult[i].Token)
			crep.Message = tokenStateCheckResult[i].Message
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		c.log.Debug("Token", tokenStateCheckResult[i].Token, "Message", tokenStateCheckResult[i].Message)
	}
	c.log.Debug("Proceeding to pin token state to prevent double spend")
	sender := cr.SenderPeerID + "." + sc.GetSenderDID()
	var receiver string

	// if receiver did is provided and is not same as sender did, then it is a normal transfer,
	// else it is a self-transfer
	if sc.GetReceiverDID() != "" && sc.GetReceiverDID() != sc.GetSenderDID() {
		receiver = cr.ReceiverPeerID + "." + sc.GetReceiverDID()
	} else {
		receiver = ""
	}
	err1 := c.pinTokenState(tokenStateCheckResult, did, cr.TransactionID, sender, receiver, float64(0))
	if err1 != nil {
		crep.Message = "Error Pinning token state" + err.Error()
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	c.log.Debug("Finished Tokenstate check")

	//get trans token block hash
	txnBlockHash, err := transTknBlock.GetHash()
	if err != nil {
		c.log.Error("failed to get trans-block-hash for credit; err", err)
		crep.Message = err.Error()
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	//quorum's signature on block hash
	quorumSignature, err := qdc.PvtSign([]byte(txnBlockHash))
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		crep.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	crep.Status = true
	crep.Message = "Conensus finished successfully"
	// crep.ShareSig = qsb
	crep.PrivSig = quorumSignature
	crep.Hash = txnBlockHash

	c.log.Debug(" ^^^^^^ checking if trans block is empty : ", transTknBlock.GetBlock() == nil)

	// // quorum pledge finality in case of pre-pledging
	// if cr.Mode == SpendableRBTTransferMode {
	// 	c.log.Debug("********** proceeding for pledge finality")
	// 	// updated token state hashes
	// 	var txnTokenHashes []string = make([]string, 0)
	// 	for _, info := range ti {
	// 		t := info.Token
	// 		blockId, _ := transTknBlock.GetBlockID(t)
	// 		tokenIDTokenStateData := t + blockId
	// 		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
	// 		tokenIDTokenStateHash, _ := c.ipfs.Add(tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
	// 		txnTokenHashes = append(txnTokenHashes, tokenIDTokenStateHash)
	// 	}
	// 	pledgeTokensMap := c.pd[cr.ReqID]
	// 	pledgeFinalityReq := UpdatePledgeRequest{
	// 		Mode:                        cr.Mode,
	// 		PledgedTokens:               pledgeTokensMap.PledgedTokens[did],
	// 		TokenChainBlock:             transTknBlock.GetBlock(),
	// 		TransferredTokenStateHashes: txnTokenHashes,
	// 		TransactionID:               cr.TransactionID,
	// 		TransactionEpoch:            cr.TransactionEpoch,
	// 	}

	// 	c.log.Debug("###33pledgefinality req : is trans block empty ? ", pledgeFinalityReq.TokenChainBlock == nil)
	// 	// pledge finality
	// response := c.UpdatePledgeToken(pledgeFinalityReq, did)
	// 	if !response.Status {
	// 		errMsg := fmt.Sprintf("failed to update pledge tokens, err : %v", response.Message)
	// 		c.log.Error(errMsg)
	// 		crep.Message = errMsg
	// 		return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 	}

	// 	c.log.Debug("********** pledge finality response : ", response)

	// 	// delete pledge tokens and consensus request map
	// 	delete(c.pd, cr.ReqID)

	// }

	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) quorumNFTSaleConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	ok, sc := c.verifyContract(cr, "")
	if !ok {
		crep.Message = "Failed to verify sender signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	//check if token has multiple pins
	ti := sc.GetTransTokenInfo()
	results := make([]MultiPinCheckRes, len(ti))
	var wg sync.WaitGroup
	for i := range ti {
		wg.Add(1)
		go c.pinCheck(ti[i].Token, i, cr.SenderPeerID, "", results, &wg)
	}
	wg.Wait()
	for i := range results {
		if results[i].Error != nil {
			c.log.Error("Error occured", "error", results[i].Error)
			crep.Message = "Error while cheking Token multiple Pins"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if results[i].Status {
			c.log.Error("Token has multiple owners", "token", results[i].Token, "owners", results[i].Owners)
			crep.Message = "Token has multiple owners"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}
	// check token ownership
	validateTokenOwnershipVar, err, _ := c.validateTokenOwnership(cr, sc, did)
	if err != nil {
		validateTokenOwnershipErrorString := fmt.Sprint(err)
		if strings.Contains(validateTokenOwnershipErrorString, "parent token is not in burnt stage") {
			crep.Message = "Token ownership check failed, err: " + validateTokenOwnershipErrorString
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		c.log.Error("Tokens ownership check failed")
		crep.Message = "Token ownership check failed, err : " + err.Error()
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	if !validateTokenOwnershipVar {
		c.log.Error("Tokens ownership check failed")
		crep.Message = "Token ownership check failed"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	/* if !c.validateTokenOwnership(cr, sc) {
		c.log.Error("Token ownership check failed")
		crep.Message = "Token ownership check failed"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	} */
	//check if token is pledgedtoken
	wt := sc.GetTransTokenInfo()

	for i := range wt {
		b := c.w.GetLatestTokenBlock(wt[i].Token, wt[i].TokenType)
		if b == nil {
			c.log.Error("pledge token check Failed, failed to get latest block")
			crep.Message = "pledge token check Failed, failed to get latest block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}

	qHash := util.CalculateHash(sc.GetBlock(), "SHA3-256")
	qsb, ppb, err := qdc.Sign(util.HexToStr(qHash))
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		crep.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	crep.Status = true
	crep.Message = "Conensus finished successfully"
	crep.ShareSig = qsb
	crep.PrivSig = ppb
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) quorumSmartContractConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, consensusRequest *ConensusRequest) *ensweb.Result {
	consensusReply := ConensusReply{
		ReqID:  consensusRequest.ReqID,
		Status: false,
	}
	if consensusRequest.ContractBlock == nil {
		c.log.Error("contract block in consensus req is nil")
		consensusReply.Message = "contract block in consensus req is nil"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	consensusContract := contract.InitContract(consensusRequest.ContractBlock, nil)
	// setup the did to verify the signature
	c.log.Debug("VEryfying the deployer signature")

	var verifyDID string

	if consensusRequest.Mode == SmartContractDeployMode {
		c.log.Debug("Fetching Deployer DID")
		verifyDID = consensusContract.GetDeployerDID()
		c.log.Debug("deployer did ", verifyDID)
	} else {
		c.log.Debug("Fetching Executor DID")
		verifyDID = consensusContract.GetExecutorDID()
		c.log.Debug("executor did ", verifyDID)
	}

	dc, err := c.SetupForienDID(verifyDID, did)
	if err != nil {
		c.log.Error("Failed to get DID for verification", "err", err)
		consensusReply.Message = "Failed to get DID for verification"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	err = consensusContract.VerifySignature(dc)
	if err != nil {
		c.log.Error("Failed to verify signature", "err", err)
		consensusReply.Message = "Failed to verify signature"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	//initiate trans token block
	transSCBlock := block.InitBlock(consensusRequest.TransTokenBlock, nil, block.NoSignature())
	if transSCBlock == nil {
		c.log.Error("Failed to do signature, invalid NFT")
		consensusReply.Message = "Failed to do signature, invalid NFT"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	// response, err := c.ValidateTxnInitiator(transSCBlock)
	// if err != nil {
	// 	c.log.Error("signature request failed, msg", response.Message, "err", err)
	// 	consensusReply.Message = response.Message
	// 	return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	// }

	//check if deployment or execution

	var tokenStateCheckResult []TokenStateCheckResult
	var wg sync.WaitGroup
	if consensusRequest.Mode == SmartContractDeployMode {
		//if deployment
		commitedTokenInfo := consensusContract.GetCommitedTokensInfo()
		//1. check commited token authenticity
		c.log.Debug("validation 1 - Authenticity of commited RBT tokens")
		validateTokenOwnershipVar, err, _ := c.validateTokenOwnership(consensusRequest, consensusContract, did)
		if err != nil {
			validateTokenOwnershipErrorString := fmt.Sprint(err)
			if strings.Contains(validateTokenOwnershipErrorString, "parent token is not in burnt stage") {
				consensusReply.Message = "Commited Token ownership check failed, err: " + validateTokenOwnershipErrorString
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
			c.log.Error("Commited Tokens ownership check failed")
			consensusReply.Message = "Commited Token ownership check failed, err : " + err.Error()
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		if !validateTokenOwnershipVar {
			c.log.Error("Commited Tokens ownership check failed")
			consensusReply.Message = "Commited Token ownership check failed"
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		//2. check commited token double spent
		c.log.Debug("validation 2 - double spent check on the commited rbt tokens")
		results := make([]MultiPinCheckRes, len(commitedTokenInfo))
		for i := range commitedTokenInfo {
			wg.Add(1)
			go c.pinCheck(commitedTokenInfo[i].Token, i, consensusRequest.DeployerPeerID, "", results, &wg)
		}
		wg.Wait()
		for i := range results {
			if results[i].Error != nil {
				c.log.Error("Error occured", "error", err)
				consensusReply.Message = "Error while cheking Token multiple Pins"
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
			if results[i].Status {
				c.log.Error("Token has multiple owners", "token", results[i].Token, "owners", results[i].Owners)
				consensusReply.Message = "Token has multiple owners"
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
		}

		//in deploy mode pin token state of commited RBT tokens
		tokenStateCheckResult = make([]TokenStateCheckResult, len(commitedTokenInfo))
		for i, ti := range commitedTokenInfo {
			t := ti.Token
			wg.Add(1)
			go c.checkTokenState(t, did, i, tokenStateCheckResult, &wg, consensusRequest.QuorumList, ti.TokenType)
		}
		wg.Wait()
	} else {
		//sync the smartcontract tokenchain
		address := consensusRequest.ExecuterPeerID + "." + consensusContract.GetExecutorDID()
		peerConn, err := c.getPeer(address)
		if err != nil {
			c.log.Error("Failed to get executor peer to sync smart contract token chain", "err", err)
			consensusReply.Message = "Failed to get executor peer to sync smart contract token chain : "
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}

		//3. check token state -- execute mode - pin tokenstate of the smart token chain
		tokenStateCheckResult = make([]TokenStateCheckResult, len(consensusContract.GetTransTokenInfo()))
		smartContractTokenInfo := consensusContract.GetTransTokenInfo()
		for i, ti := range smartContractTokenInfo {
			t := ti.Token
			err = c.syncTokenChainFrom(peerConn, "", ti.Token, ti.TokenType)
			if err != nil {
				c.log.Error("Failed to sync smart contract token chain block fro execution validation", "err", err)
				consensusReply.Message = "Failed to sync smart contract token chain block fro execution validation"
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
			wg.Add(1)
			go c.checkTokenState(t, did, i, tokenStateCheckResult, &wg, consensusRequest.QuorumList, ti.TokenType)
		}
		wg.Wait()
	}
	for i := range tokenStateCheckResult {
		if tokenStateCheckResult[i].Error != nil {
			c.log.Error("Error occured", "error", err)
			consensusReply.Message = "Error while cheking Token State Message : " + tokenStateCheckResult[i].Message
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		if tokenStateCheckResult[i].Exhausted {
			c.log.Debug("Token state has been exhausted, Token being Double spent:", tokenStateCheckResult[i].Token)
			consensusReply.Message = tokenStateCheckResult[i].Message
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		c.log.Debug("Token", tokenStateCheckResult[i].Token, "Message", tokenStateCheckResult[i].Message)
	}

	c.log.Debug("Proceeding to pin token state to prevent double spend")
	err = c.pinTokenState(tokenStateCheckResult, did, consensusRequest.TransactionID, "NA", "NA", float64(0)) // TODO: Ensure that smart contract trnx id and things are proper
	if err != nil {
		consensusReply.Message = "Error Pinning token state" + err.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	c.log.Debug("Finished Tokenstate check")

	//get trans token block hash
	txnBlockHash, err := transSCBlock.GetHash()
	if err != nil {
		c.log.Error("failed to get trans-block-hash for credit; err", err)
		consensusReply.Message = err.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	//quorum's signature on block hash
	qsb, ppb, err := qdc.Sign(txnBlockHash)
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		consensusReply.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	consensusReply.Status = true
	consensusReply.Message = "Consensus finished successfully"
	consensusReply.ShareSig = qsb
	consensusReply.PrivSig = ppb
	consensusReply.Hash = txnBlockHash
	return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
}

func (c *Core) quorumNFTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, consensusRequest *ConensusRequest) *ensweb.Result {
	consensusReply := ConensusReply{
		ReqID:  consensusRequest.ReqID,
		Status: false,
	}
	if consensusRequest.ContractBlock == nil {
		c.log.Error("contract block in consensus req is nil")
		consensusReply.Message = "contract block in consensus req is nil"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	consensusContract := contract.InitContract(consensusRequest.ContractBlock, nil)
	// setup the did to verify the signature
	c.log.Info("Verifying the deployer signature while deploying nft")

	var verifyDID string

	if consensusRequest.Mode == NFTDeployMode {
		c.log.Debug("Fetching NFT Deployer DID")
		verifyDID = consensusContract.GetDeployerDID()
		c.log.Debug("deployer did ", verifyDID)
	} else {
		c.log.Debug("Fetching NFT Executor DID")
		verifyDID = consensusContract.GetExecutorDID()
		c.log.Debug("Executor did ", verifyDID)
	}

	dc, err := c.SetupForienDID(verifyDID, did)
	if err != nil {
		c.log.Error("Failed to get DID for verification", "err", err)
		consensusReply.Message = "Failed to get DID for verification"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	err = consensusContract.VerifySignature(dc)
	if err != nil {
		c.log.Error("Failed to verify signature", "err", err)
		consensusReply.Message = "Failed to verify signature"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	//initiate trans token block
	transNFTBlock := block.InitBlock(consensusRequest.TransTokenBlock, nil, block.NoSignature())
	if transNFTBlock == nil {
		c.log.Error("Failed to do signature, invalid NFT")
		consensusReply.Message = "Failed to do signature, invalid NFT"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	// //Validate sender signature
	// response, err := c.ValidateTxnInitiator(transNFTBlock)
	// if err != nil {
	// 	c.log.Error("signature request failed, msg", response.Message, "err", err)
	// 	consensusReply.Message = response.Message
	// 	return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	// }

	//check if deployment or execution

	var tokenStateCheckResult []TokenStateCheckResult
	var wg sync.WaitGroup
	if consensusRequest.Mode == NFTDeployMode {
		//if deployment
		commitedTokenInfo := consensusContract.GetCommitedTokensInfo()
		//1. check commited token authenticity
		c.log.Debug("validation 1 - Authenticity of commited RBT tokens")
		validateTokenOwnershipVar, err, _ := c.validateTokenOwnership(consensusRequest, consensusContract, did)
		if err != nil {
			validateTokenOwnershipErrorString := fmt.Sprint(err)
			if strings.Contains(validateTokenOwnershipErrorString, "parent token is not in burnt stage") {
				consensusReply.Message = "Commited Token ownership check failed, err: " + validateTokenOwnershipErrorString
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
			c.log.Error("Commited Tokens ownership check failed")
			consensusReply.Message = "Commited Token ownership check failed, err : " + err.Error()
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		if !validateTokenOwnershipVar {
			c.log.Error("Commited Tokens ownership check failed")
			consensusReply.Message = "Commited Token ownership check failed"
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		//2. check commited token double spent
		c.log.Debug("validation 2 - double spent check on the commited rbt tokens")
		results := make([]MultiPinCheckRes, len(commitedTokenInfo))
		for i := range commitedTokenInfo {
			wg.Add(1)
			go c.pinCheck(commitedTokenInfo[i].Token, i, consensusRequest.DeployerPeerID, "", results, &wg)
		}
		wg.Wait()
		for i := range results {
			if results[i].Error != nil {
				c.log.Error("Error occured", "error", err)
				consensusReply.Message = "Error while cheking Token multiple Pins"
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
			if results[i].Status {
				c.log.Error("Token has multiple owners", "token", results[i].Token, "owners", results[i].Owners)
				consensusReply.Message = "Token has multiple owners"
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
		}

		//in deploy mode pin token state of commited RBT tokens
		tokenStateCheckResult = make([]TokenStateCheckResult, len(commitedTokenInfo))
		for i, ti := range commitedTokenInfo {
			t := ti.Token
			wg.Add(1)
			go c.checkTokenState(t, did, i, tokenStateCheckResult, &wg, consensusRequest.QuorumList, ti.TokenType)
		}
		wg.Wait()
	} else {
		//sync the nft tokenchain
		address := consensusRequest.ExecuterPeerID + "." + consensusContract.GetExecutorDID()
		peerConn, err := c.getPeer(address)
		if err != nil {
			c.log.Error("Failed to get executor peer to sync NFT chain", "err", err)
			consensusReply.Message = "Failed to get executor peer to sync NFT token chain : "
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}

		//3. check token state -- execute mode - pin tokenstate of the nft
		tokenStateCheckResult = make([]TokenStateCheckResult, len(consensusContract.GetTransTokenInfo()))
		nftInfo := consensusContract.GetTransTokenInfo()
		for i, ti := range nftInfo {
			t := ti.Token
			err = c.syncTokenChainFrom(peerConn, "", ti.Token, ti.TokenType)
			if err != nil {
				c.log.Error("Failed to sync nft chain block from execution validation", "err", err)
				consensusReply.Message = "Failed to sync nft chain block from execution validation"
				return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
			}
			wg.Add(1)
			go c.checkTokenState(t, did, i, tokenStateCheckResult, &wg, consensusRequest.QuorumList, ti.TokenType)
		}
		wg.Wait()
	}
	for i := range tokenStateCheckResult {
		if tokenStateCheckResult[i].Error != nil {
			c.log.Error("Error occured", "error", err)
			consensusReply.Message = "Error while checking Token State Message while deploying nft : " + tokenStateCheckResult[i].Message
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		if tokenStateCheckResult[i].Exhausted {
			c.log.Debug("Token state has been exhausted, Token being Double spent:", tokenStateCheckResult[i].Token)
			consensusReply.Message = tokenStateCheckResult[i].Message
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		c.log.Debug("Token", tokenStateCheckResult[i].Token, "Message", tokenStateCheckResult[i].Message)
	}

	c.log.Debug("Proceeding to pin token state to prevent double spend")
	err = c.pinTokenState(tokenStateCheckResult, did, consensusRequest.TransactionID, "NA", "NA", float64(0)) // TODO: Ensure that smart contract trnx id and things are proper
	if err != nil {
		consensusReply.Message = "Error Pinning token state" + err.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	c.log.Debug("Finished Tokenstate check")

	//get trans token block hash
	txnBlockHash, err := transNFTBlock.GetHash()
	if err != nil {
		c.log.Error("failed to get trans-block-hash for credit; err", err)
		consensusReply.Message = err.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	//quorum's signature on block hash
	qsb, ppb, err := qdc.Sign(txnBlockHash)
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		consensusReply.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	consensusReply.Status = true
	consensusReply.Message = "Consensus finished successfully"
	consensusReply.ShareSig = qsb
	consensusReply.PrivSig = ppb
	consensusReply.Hash = txnBlockHash
	return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
}

func (c *Core) quorumFTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, consensusRequest *ConensusRequest) *ensweb.Result {
	consensusReply := ConensusReply{
		ReqID:  consensusRequest.ReqID,
		Status: false,
	}

	ok, sc := c.verifyContract(consensusRequest, did)
	if !ok {
		consensusReply.Message = "Failed to verify sender signature"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	transFTBlock := block.InitBlock(consensusRequest.TransTokenBlock, nil, block.NoSignature())
	if transFTBlock == nil {
		c.log.Error("Failed to do signature, invalid NFT")
		consensusReply.Message = "Failed to do signature, invalid NFT"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	ti := sc.GetTransTokenInfo()

	pinCheckResults := make([]MultiPinCheckRes, len(ti))
	tokenStateCheckResults := make([]TokenStateCheckResult, len(ti))

	// === Parallel Pin Check ===
	// This section remains similar to your original, but using pinWG

	// UPDATE: Token Bucket size update here
	pinBucketSize := 20 // Tunable bucket size for pin checks
	pinConcurrency := runtime.NumCPU()
	pinTokenBuckets := c.createTokenBuckets(ti, pinBucketSize)
	var pinWG sync.WaitGroup // Dedicated WaitGroup for Pin Checks
	pinErrChan := make(chan error, len(pinTokenBuckets))
	pinSem := make(chan struct{}, pinConcurrency) // Semaphore for pin checks

	for _, bucket := range pinTokenBuckets {
		pinWG.Add(1) // Add for the goroutine that processes this bucket
		pinSem <- struct{}{}
		go func(toks []contract.TokenInfo) {
			defer pinWG.Done() // Done for the bucket goroutine
			defer func() { <-pinSem }()

			for _, t := range toks {
				var err error
				for attempt := 0; attempt < 3; attempt++ {
					err = c.pinCheckWorker(t, consensusRequest.SenderPeerID, consensusRequest.ReceiverPeerID, pinCheckResults, ti)
					if err == nil || !isRecoverableError(err) {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
				if err != nil {
					pinErrChan <- fmt.Errorf("pin check failed for token %s: %w", t.Token, err)
					// No return here, allow other tokens in the same bucket to be processed
				}
			}
		}(bucket)
	}
	pinWG.Wait() // Wait for all pin check goroutines to finish
	close(pinErrChan)

	for err := range pinErrChan {
		c.log.Error("Error occurred during pin check", "error", err)
		consensusReply.Message = "Error while checking FT Token multiple Pins: " + err.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	for i := range pinCheckResults {
		if pinCheckResults[i].Status { // If Status is true, it means multiple owners
			c.log.Error("FT Token has multiple owners", "FT token", pinCheckResults[i].Token, "owners", pinCheckResults[i].Owners)
			consensusReply.Message = "FT Token has multiple owners: " + pinCheckResults[i].Token
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
	}

	// === Token Ownership Check ===
	c.log.Debug("*********** starting validateTokenOwnership")
	validateTokenOwnershipVar, err, syncIssueTokens := c.validateTokenOwnershipInBatch(consensusRequest, sc, did)
	if len(syncIssueTokens) > 0 {

		consensusReply.Result = syncIssueTokens
	}
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "parent token is not in burnt stage") || strings.Contains(errStr, "failed to sync tokenchain Token") {
			consensusReply.Message = "Token ownership check failed, err: " + errStr
			return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
		}
		c.log.Error("Tokens ownership check failed")
		consensusReply.Message = "Token ownership check failed, err : " + errStr
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}
	if !validateTokenOwnershipVar {
		c.log.Error("Tokens ownership check failed")
		consensusReply.Message = "Token ownership check failed"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	// === Token State Check (Parallel) ===
	// This section is now distinct and uses its own tokenStateWG

	// UPDATE: Token Bucket size update here
	tokenStateBucketSize := 20 // Tunable bucket size for token state checks
	tokenStateConcurrency := runtime.NumCPU()
	tokenStateTokenBuckets := c.createTokenBuckets(ti, tokenStateBucketSize)
	var tokenStateWG sync.WaitGroup                                                         // Dedicated WaitGroup for Token State Checks
	tokenStateErrChan := make(chan error, len(tokenStateTokenBuckets)*tokenStateBucketSize) // Max errors = total tokens
	tokenStateSem := make(chan struct{}, tokenStateConcurrency)                             // Semaphore for token state checks

	// Loop over buckets to launch goroutines
	for _, bucket := range tokenStateTokenBuckets {
		// Each goroutine will handle a bucket of tokens.
		// For each token *within* the bucket, `checkTokenState` will call `wg.Done()`.
		// So, `tokenStateWG.Add()` needs to be called for each token, NOT each bucket goroutine.
		// We'll manage this by adding inside the goroutine if `checkTokenState` must call Done().
		tokenStateSem <- struct{}{} // Acquire semaphore slot for this bucket goroutine
		go func(toks []contract.TokenInfo) {
			defer func() { <-tokenStateSem }() // Release semaphore slot when this bucket goroutine finishes

			for _, t := range toks {
				tokenStateWG.Add(1) // ADD FOR EACH TOKEN, as checkTokenState calls Done() for each token
				// Call checkTokenState directly. It has its own retry logic (if applicable) and writes to results.
				// The wg parameter here refers to the tokenStateWG in the outer scope.
				var checkErr error // To capture errors if a retry fails
				for attempt := 0; attempt < 3; attempt++ {
					c.checkTokenState(t.Token, did, indexOfToken(ti, t.Token), tokenStateCheckResults, &tokenStateWG, consensusRequest.QuorumList, t.TokenType)
					// After checkTokenState returns, its internal defer wg.Done() has fired.
					// We need to check the error it wrote to the resultArray
					checkErr = tokenStateCheckResults[indexOfToken(ti, t.Token)].Error
					if checkErr == nil || !isRecoverableError(checkErr) {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}

				if checkErr != nil {
					tokenStateErrChan <- fmt.Errorf("token state check failed for token %s: %w", t.Token, checkErr)
				} else if tokenStateCheckResults[indexOfToken(ti, t.Token)].Exhausted {
					tokenStateErrChan <- fmt.Errorf("token %s is exhausted: %s", t.Token, tokenStateCheckResults[indexOfToken(ti, t.Token)].Message)
				}
				// No `return` here, allow other tokens in the same bucket to be processed.
			}
		}(bucket)
	}

	tokenStateWG.Wait() // Wait for all individual token state checks to complete
	close(tokenStateErrChan)

	for err := range tokenStateErrChan {
		c.log.Error("Error occurred during token state check", "error", err)
		consensusReply.Message = "Error while checking Token State: " + err.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	// === Pin Token State ===
	c.log.Debug("Proceeding to pin token state to prevent double spend")
	sender := consensusRequest.SenderPeerID + "." + sc.GetSenderDID()
	receiver := consensusRequest.ReceiverPeerID + "." + sc.GetReceiverDID()
	err1 := c.pinTokenState(tokenStateCheckResults, did, consensusRequest.TransactionID, sender, receiver, float64(0))
	if err1 != nil {
		consensusReply.Message = "Error Pinning token state: " + err1.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	// === Sign Final Block ===
	txnBlockHash, err := transFTBlock.GetHash()
	if err != nil {
		c.log.Error("failed to get trans-block-hash for credit; err", err)
		consensusReply.Message = err.Error()
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	qsb, ppb, err := qdc.Sign(txnBlockHash)
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		consensusReply.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	consensusReply.Status = true
	consensusReply.Message = "FT Consensus finished successfully"
	consensusReply.ShareSig = qsb
	consensusReply.PrivSig = ppb
	consensusReply.Hash = txnBlockHash
	return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
}

func (c *Core) pinCheckWorker(token contract.TokenInfo, senderID, receiverID string, results []MultiPinCheckRes, ti []contract.TokenInfo) error {
	var wg sync.WaitGroup
	wg.Add(1)
	idx := indexOfToken(ti, token.Token)
	c.pinCheck(token.Token, idx, senderID, receiverID, results, &wg)
	wg.Wait()
	return results[idx].Error
}

func (c *Core) createTokenBuckets(ti []contract.TokenInfo, bucketSize int) [][]contract.TokenInfo {
	var buckets [][]contract.TokenInfo
	for i := 0; i < len(ti); i += bucketSize {
		end := i + bucketSize
		if end > len(ti) {
			end = len(ti)
		}
		buckets = append(buckets, ti[i:end])
	}
	return buckets
}

func indexOfToken(tokens []contract.TokenInfo, id string) int {
	for i, t := range tokens {
		if t.Token == id {
			return i
		}
	}
	return -1
}

func isRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "ipfs") ||
		strings.Contains(errMsg, "network") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "EOF") ||
		strings.Contains(errMsg, "connection reset")
}

func (c *Core) quorumConensus(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var cr ConensusRequest
	err := c.l.ParseJSON(req, &cr)
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse request", "err", err)
		crep.Message = "Failed to parse request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	qdc, ok := c.qc[did]
	if !ok {
		c.log.Error("Quorum is not setup")
		crep.Message = "Quorum is not setup"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	switch cr.Mode {
	case RBTTransferMode, SelfTransferMode, PinningServiceMode, SpendableRBTTransferMode:
		c.log.Debug("RBT consensus started")
		return c.quorumRBTConsensus(req, did, qdc, &cr)
	case DTCommitMode:
		c.log.Debug("Data consensus started")
		return c.quorumDTConsensus(req, did, qdc, &cr)
	case NFTSaleContractMode:
		c.log.Debug("NFT sale contract started")
		return c.quorumNFTSaleConsensus(req, did, qdc, &cr)
	case SmartContractDeployMode:
		c.log.Debug("Smart contract Consensus for Deploy started")
		return c.quorumSmartContractConsensus(req, did, qdc, &cr)
	case SmartContractExecuteMode:
		c.log.Debug("Smart contract Consensus for execution started")
		return c.quorumSmartContractConsensus(req, did, qdc, &cr)
	case NFTDeployMode:
		c.log.Info("NFT deploy consensus started")
		return c.quorumNFTConsensus(req, did, qdc, &cr)
	case NFTExecuteMode:
		c.log.Info("NFT execute consensus started")
		return c.quorumNFTConsensus(req, did, qdc, &cr)
	case FTTransferMode, SpendableFTTransferMode:
		c.log.Debug("FT consensus started")
		return c.quorumFTConsensus(req, did, qdc, &cr)
	default:
		c.log.Error("Invalid consensus mode", "mode", cr.Mode)
		crep.Message = "Invalid consensus mode"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
}

func (c *Core) reqPledgeToken(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var pr PledgeRequest
	err := c.l.ParseJSON(req, &pr)
	crep := model.BasicResponse{
		Status: false,
	}
	c.log.Debug("**********Request for pledge, crReqId : ", pr.ConensusRequestID, "pledge amount ", pr.TokensRequired)
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	_, ok := c.qc[did]
	if !ok {
		c.log.Error("Quorum is not setup")
		crep.Message = "Quorum is not setup"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	var availableBalance model.DIDAccountInfo
	availableBalance, err = c.GetAccountInfo(did)
	if err != nil {
		c.log.Error("Unable to check quorum balance")
	}

	c.log.Debug("********* available balance : ", availableBalance)

	availableRBT := availableBalance.RBTAmount
	if availableRBT < pr.TokensRequired {
		c.log.Error("Quorum don't have enough balance to pledge")
		crep.Message = "Quorum don't have enough balance to pledge"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	if (pr.TokensRequired) < MinDecimalValue(MaxDecimalPlaces) {
		c.log.Error("Pledge amount is less than ", MinDecimalValue(MaxDecimalPlaces))
		crep.Message = "Pledge amount is less than minimum transcation amount"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	decimalPlaces := strconv.FormatFloat(pr.TokensRequired, 'f', -1, 64)
	decimalPlacesStr := strings.Split(decimalPlaces, ".")
	if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > MaxDecimalPlaces {
		c.log.Error("Pledge amount exceeds %d decimal places.\n", MaxDecimalPlaces)
		crep.Message = fmt.Sprintf("Pledge amount exceeds %d decimal places.\n", MaxDecimalPlaces)
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	dc := c.pqc[did]
	wt, err := c.GetTokens(dc, did, pr.TokensRequired, RBTTransferMode)
	if err != nil {
		crep.Message = "Failed to get tokens"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	totalLockedTokens := len(wt)
	if totalLockedTokens == 0 {
		c.log.Error("No tokens left to pledge", "err", err)
		crep.Message = "No tokens left to pledge"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	presp := PledgeReply{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got available tokens",
		},
		Tokens:          make([]string, 0),
		TokenValue:      make([]float64, 0),
		TokenChainBlock: make([][]byte, 0),
	}

	var totalPledgeAmt float64 = 0

	for i := 0; i < totalLockedTokens; i++ {
		presp.Tokens = append(presp.Tokens, wt[i].TokenID)
		presp.TokenValue = append(presp.TokenValue, wt[i].TokenValue)
		totalPledgeAmt = floatPrecision(totalPledgeAmt, MaxDecimalPlaces) + wt[i].TokenValue
		ts := RBTString
		if wt[i].TokenValue != 1.0 {
			ts = PartString
		}
		tc := c.w.GetLatestTokenBlock(wt[i].TokenID, c.TokenType(ts))
		if tc == nil {
			c.log.Error("Failed to get latest token chain block")
			crep.Message = "Failed to get latest token chain block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		presp.TokenChainBlock = append(presp.TokenChainBlock, tc.GetBlock())
	}

	c.log.Debug("****** pledge reply :", presp.TokenValue)
	c.log.Debug("********* total pledge amt ", floatPrecision(totalPledgeAmt, MaxDecimalPlaces))

	// maintain pledge tokens map with conensus request ID in core struct for future reference
	consensusPledgeTknMap := make(map[string][]string)
	consensusPledgeTknMap[did] = presp.Tokens
	c.pd[pr.ConensusRequestID] = &PledgeDetails{
		PledgedTokens:    consensusPledgeTknMap,
		NumPledgedTokens: totalLockedTokens,
	}

	c.log.Debug("********* pledge tokens map : ", c.pd[pr.ConensusRequestID])

	return c.l.RenderJSON(req, &presp, http.StatusOK)
}

func (c *Core) updateReceiverToken(
	senderAddress string, receiverAddress string, tokenInfo []contract.TokenInfo, tokenChainBlock []byte,
	quorumList []string, quorumInfo []QuorumDIDPeerMap, CVRStage int, transactionEpoch int, pinningServiceMode bool,
) ([]string, *ipfsport.Peer, error) {
	var receiverPeerId string = ""
	var receiverDID string = ""

	if receiverAddress != "" {
		var ok bool
		receiverPeerId, receiverDID, ok = util.ParseAddress(receiverAddress)
		if !ok {
			return nil, nil, fmt.Errorf("unable to parse receiver address: %v", receiverAddress)
		}
	} else {
		var ok bool
		receiverPeerId, receiverDID, ok = util.ParseAddress(senderAddress)
		if !ok {
			return nil, nil, fmt.Errorf("unable to parse receiver address: %v", senderAddress)
		}
	}

	var updatedTokenStateHashes []string

	b := block.InitBlock(tokenChainBlock, nil, block.NoSignature())
	if b == nil {
		return nil, nil, fmt.Errorf("invalid token chain block")
	}

	var senderPeer *ipfsport.Peer
	senderPeerId, _, ok := util.ParseAddress(senderAddress)
	if !ok {
		return nil, senderPeer, fmt.Errorf("unable to parse sender address: %v", senderAddress)
	}

	if receiverAddress != "" && CVRStage == wallet.CVRStage1_Sender_to_Receiver {

		c.log.Debug("*********** updating tokens in cvr-1************")

		var err error
		senderPeer, err = c.getPeer(senderAddress)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get peer : %v", err.Error())
		}
		// defer senderPeer.Close()
		for _, ti := range tokenInfo {
			t := ti.Token
			pblkID, err := b.GetPrevBlockID(t)
			if err != nil {
				return nil, senderPeer, fmt.Errorf("failed to sync token chain block, missing previous block id for token %v, error: %v", t, err)
			}

			err = c.syncTokenChainFrom(senderPeer, pblkID, t, ti.TokenType)
			if err != nil {
				// Add return of syncIssueTokenArray
				c.log.Error("receiver failed to sync token chain of token ", ti.Token, "error ", err)
				return nil, senderPeer, fmt.Errorf("failed to sync tokenchain Token: %v, issueType: %v", t, TokenChainNotSynced)
			}

			if c.TokenType(PartString) == ti.TokenType {
				gb := c.w.GetGenesisTokenBlock(t, ti.TokenType)
				if gb == nil {
					return nil, senderPeer, fmt.Errorf("failed to get genesis block for token %v, err: %v", t, err)
				}
				pt, _, err := gb.GetParentDetials(t)
				if err != nil {
					return nil, senderPeer, fmt.Errorf("failed to get parent details for token %v, err: %v", t, err)
				}
				_, err = c.syncParentToken(senderPeer, pt)
				if err != nil {
					return nil, senderPeer, fmt.Errorf("failed to sync parent token %v childtoken %v err : %v", pt, t, err)
				}

			}
			ptcbArray, err := c.w.GetTokenBlock(t, ti.TokenType, pblkID)
			if err != nil {
				return nil, senderPeer, fmt.Errorf("failed to fetch previous block for token: %v err : %v", t, err)
			}
			ptcb := block.InitBlock(ptcbArray, nil)
			if c.checkIsPledged(ptcb) {
				return nil, senderPeer, fmt.Errorf("Token " + t + " is a pledged Token")
			}

			c.log.Debug("************ updating token type, adding 50 to the current type")

			// updating token type as per cvr stage
			b, ok = b.UpdateTokenType(t, token.CVR_RBTTokenType+ti.TokenType)
			if !ok {
				c.log.Error("failed to update token cvr type")
				return nil, senderPeer, fmt.Errorf("failed to update tokenType of  Token " + t)
			}

		}
		//TODO: handle the sync status, as aliredy synced in cvr-1, can update the synce status to completed in cvrstage-2
		updatedTokenStateHashes, err = c.w.TokensReceived(receiverDID, tokenInfo, b, senderPeerId, receiverPeerId, pinningServiceMode, c.ipfs)
		if err != nil {
			return nil, senderPeer, fmt.Errorf("failed to update token status, error: %v", err)
		}

		c.log.Debug("*********** updated tokemn state hahses : ", updatedTokenStateHashes)

		sc := contract.InitContract(b.GetSmartContract(), nil)
		if sc == nil {
			return nil, senderPeer, fmt.Errorf("failed to update token status, missing smart contract")
		}

		bid, err := b.GetBlockID(tokenInfo[0].Token)
		if err != nil {
			return nil, senderPeer, fmt.Errorf("failed to update token status, failed to get block ID, err: %v", err)
		}

		// Store the transaction info only when we are dealing with RBT transfer between
		// two DIDs that are situated on different nodes, as this avoid Unique Constraint
		// issue while adding to Transaction History table from the Sender's end
		if sc.GetSenderDID() != sc.GetReceiverDID() && senderPeerId != receiverPeerId {
			td := &model.TransactionDetails{
				TransactionID:   b.GetTid(),
				TransactionType: b.GetTransType(),
				BlockID:         bid,
				Mode:            wallet.RecvMode,
				Amount:          sc.GetTotalRBTs(),
				SenderDID:       sc.GetSenderDID(),
				ReceiverDID:     sc.GetReceiverDID(),
				Comment:         sc.GetComment(),
				DateTime:        time.Now(),
				Status:          true,
				Epoch:           int64(transactionEpoch),
			}
			if td.Epoch == 0 {
				td.Epoch = time.Now().Unix()
			}

			c.log.Debug("************ adding transaction details to DB : ", td)

			c.w.AddTransactionHistory(td)
		}
		//updating the token type in cvr stage1

	}

	if CVRStage == wallet.CVRStage2_Sender_to_Receiver {

		c.log.Debug("********** updating token in cvr-2 *****************")

		results := make([]MultiPinCheckRes, len(tokenInfo))
		var wg sync.WaitGroup
		for i, ti := range tokenInfo {
			t := ti.Token

			c.log.Debug("************* removing cvr-1 block, token : ", t, " token type ", ti.TokenType)

			// if latest block is cvr-1 block, remove it before adding cvr-2 block
			err := c.RemoveSpendableRBTTransferredBlock(t, ti.TokenType)
			if err != nil {
				// TODO : if failed to remove cvr-1 block then store the info in DB and retry it
				c.log.Error(err.Error())
				return nil, nil, fmt.Errorf(err.Error())
			}

			wg.Add(1)

			go c.pinCheck(t, i, senderPeerId, receiverPeerId, results, &wg)
		}
		wg.Wait()

		for i := range results {
			if results[i].Error != nil {
				return nil, senderPeer, fmt.Errorf("error while cheking Token multiple Pins for token %v, error : %v", results[i].Token, results[i].Error)
			}
			if results[i].Status {
				return nil, senderPeer, fmt.Errorf("token %v has multiple owners: %v", results[i].Token, results[i].Owners)
			}
		}

		tokenStateCheckResult := make([]TokenStateCheckResult, len(tokenInfo))
		for i, ti := range tokenInfo {
			t := ti.Token
			wg.Add(1)
			go c.checkTokenState(t, receiverDID, i, tokenStateCheckResult, &wg, quorumList, ti.TokenType)
		}
		wg.Wait()

		for i := range tokenStateCheckResult {
			if tokenStateCheckResult[i].Error != nil {
				return nil, senderPeer, fmt.Errorf("error while cheking Token State Message : %v", tokenStateCheckResult[i].Message)
			}
			if tokenStateCheckResult[i].Exhausted {
				c.log.Debug("Token state has been exhausted, Token being Double spent:", tokenStateCheckResult[i].Token)
				return nil, senderPeer, fmt.Errorf("token state has been exhausted, Token being Double spent: %v, msg: %v", tokenStateCheckResult[i].Token, tokenStateCheckResult[i].Message)
			}
			c.log.Debug("Token", tokenStateCheckResult[i].Token, "Message", tokenStateCheckResult[i].Message)
			c.log.Debug("************* token type is :", tokenInfo[i].TokenType)
		}
		var err error
		updatedTokenStateHashes, err = c.w.TokensReceived(receiverDID, tokenInfo, b, senderPeerId, receiverPeerId, pinningServiceMode, c.ipfs)
		if err != nil {
			return nil, senderPeer, fmt.Errorf("failed to update token status, error: %v", err)
		}
		//Adding quorums to DIDPeerTable of receiver
		for _, qrm := range quorumInfo {
			c.w.AddDIDPeerMap(qrm.DID, qrm.PeerID, *qrm.DIDType)
		}
	}

	return updatedTokenStateHashes, senderPeer, nil
}

func (c *Core) updateReceiverTokenHandle(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var sr SendTokenRequest

	c.log.Debug("********* updatinhg receiver tokens, receiver : ", did, "tokens : ", sr.TokenInfo, " cvr stage ", sr.CVRStage)

	err := c.l.ParseJSON(req, &sr)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	receiverAddress := c.peerID + "." + did
	updatedtokenhashes, senderPeer, err := c.updateReceiverToken(
		sr.Address,
		receiverAddress,
		sr.TokenInfo,
		sr.TokenChainBlock,
		sr.QuorumList,
		sr.QuorumInfo,
		sr.CVRStage,
		sr.TransactionEpoch,
		sr.PinningServiceMode,
	)
	if err != nil {
		c.log.Error(err.Error())
		crep.Message = err.Error()
		if senderPeer != nil {
			senderPeer.Close()
		}
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	if sr.CVRStage == wallet.CVRStage1_Sender_to_Receiver {
		// // receiver fetches tokens to be synced
		// tokensSyncInfo := make([]TokenSyncInfo, 0)
		// for _, token := range sr.TokenInfo {
		// 	tokensSyncInfo = append(tokensSyncInfo, TokenSyncInfo{TokenID: token.Token, TokenType: token.TokenType})
		// 	if token.TokenType == c.TokenType(PartString) {
		// 		tokenInfo, err := c.w.ReadToken(token.Token)
		// 		if err != nil {
		// 			c.log.Error("failed to fetch parent token info, err ", err)
		// 			//TODO : handle the situation when not able to fetch parent tokenId to sync parent token chain
		// 			continue
		// 		}
		// 		// parentTokenId := tokenInfo.ParentTokenID
		// 		parentTokenInfo, err := c.w.ReadToken(tokenInfo.ParentTokenID)
		// 		if err != nil {
		// 			c.log.Error("failed to fetch parent token value, err ", err)
		// 			// update token sync status
		// 			c.w.UpdateTokenSyncStatus(tokenInfo.ParentTokenID, wallet.SyncIncomplete)
		// 			continue
		// 		}
		// 		if parentTokenInfo.TokenValue != 1.0 {
		// 			tokensSyncInfo = append(tokensSyncInfo, TokenSyncInfo{TokenID: tokenInfo.ParentTokenID, TokenType: c.TokenType(PartString)})
		// 		} else {
		// 			tokensSyncInfo = append(tokensSyncInfo, TokenSyncInfo{TokenID: tokenInfo.ParentTokenID, TokenType: c.TokenType(RBTString)})
		// 		}
		// 	}
		// }
		// tokenSyncMap := make(map[string][]TokenSyncInfo)
		// tokenSyncMap[senderPeer.GetPeerID()+"."+senderPeer.GetPeerDID()] = tokensSyncInfo

		// // syncing starts in the background
		// go c.syncFullTokenChains(tokenSyncMap)

		crep.Status = true
		crep.Message = "Token received successfully"
		c.log.Debug("cvr-1 completed, ", crep.Message)
		crep.Result = updatedtokenhashes

	} else {
		crep.Status = true
		crep.Message = "CVR status updated to 2"
		c.log.Debug("cvr-2 completed, ", crep.Message)
		crep.Result = updatedtokenhashes
	}

	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) updateFTToken(senderAddress string, receiverAddress string, tokenInfo []contract.TokenInfo, tokenChainBlock []byte,
	quorumList []string, quorumInfo []QuorumDIDPeerMap, transactionEpoch int, ftinfo *model.FTInfo, cvrStage int) ([]string, []string, error) {

	receiverPeerId, receiverDID, ok := util.ParseAddress(receiverAddress)
	b := block.InitBlock(tokenChainBlock, nil, block.NoSignature())

	// Debugging block initialization
	if b == nil {
		c.log.Error("Failed to initialize block from tokenChainBlock. Check tokenChainBlock structure.")
		return nil, nil, fmt.Errorf("invalid ft-token chain block")
	}
	var senderPeer *ipfsport.Peer
	var err error
	senderPeer, err = c.getPeer(senderAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get peer : %v", err.Error())
	}
	defer senderPeer.Close()
	var syncIssueTokens []string

	senderPeerId, _, ok := util.ParseAddress(senderAddress)
	if !ok {
		return nil, nil, fmt.Errorf("Unable to parse sender address: %v", senderAddress)
	}

	updatedTokenStateHashes := make([]string, 0)

	// In cvr-1, sync all trans-ft chains, update token type, add new ft block to token chain and
	// update FT status, add transaction to transaction history table
	if cvrStage == wallet.CVRStage1_Sender_to_Receiver {
		for _, ti := range tokenInfo {
			t := ti.Token
			pblkID, err := b.GetPrevBlockID(t)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to sync token chain block, missing previous block id for token %v, error: %v", t, err)
			}

			err = c.syncTokenChainFrom(senderPeer, pblkID, t, ti.TokenType)
			if err != nil && !strings.Contains(err.Error(), "syncer block height discrepency") {
				c.log.Error("receiver failed to sync token chain of FT ", ti.Token, "error ", err)
				// return nil, fmt.Errorf("failed to sync tokenchain Token: %v, issueType: %v", t, TokenChainNotSynced)
				syncIssueTokens = append(syncIssueTokens, t)
				continue
			} else {
				err = nil
			}

			if c.TokenType(PartString) == ti.TokenType {
				gb := c.w.GetGenesisTokenBlock(t, ti.TokenType)
				if gb == nil {
					return nil, nil, fmt.Errorf("failed to get genesis block for token %v, err: %v", t, err)
				}
				pt, _, err := gb.GetParentDetials(t)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to get parent details for token %v, err: %v", t, err)
				}
				_, err = c.syncParentToken(senderPeer, pt)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to sync parent token %v childtoken %v err %v : ", pt, t, err)
				}
			}
			ptcbArray, err := c.w.GetTokenBlock(t, ti.TokenType, pblkID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to fetch previous block for token: %v err : %v", t, err)
			}
			ptcb := block.InitBlock(ptcbArray, nil)
			if c.checkIsPledged(ptcb) {
				return nil, nil, fmt.Errorf("Token " + t + " is a pledged Token")
			}

			// updating token type as per cvr stage
			b, ok = b.UpdateTokenType(t, token.CVR_RBTTokenType+ti.TokenType)
			if !ok {
				c.log.Error("failed to update token cvr type")
				return nil, nil, fmt.Errorf("failed to update tokenType of  Token " + t)
			}
		}

		// update received token chain and table
		var FT wallet.FTToken
		FT.FTName = ftinfo.FTName
		updatedTokenStateHashes, err = c.w.FTTokensReceived(receiverDID, tokenInfo, b, senderPeerId, receiverPeerId, c.ipfs, FT)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to update token status, error: %v", err)
		}

		// add to transaction history table
		sc := contract.InitContract(b.GetSmartContract(), nil)
		if sc == nil {
			return nil, nil, fmt.Errorf("Failed to update token status, missing smart contract")
		}
		bid, err := b.GetBlockID(tokenInfo[0].Token)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to update token status, failed to get block ID, err: %v", err)
		}
		// Only save the transaction details in Transaction history table whenever
		// its a general RBT transfer
		if sc.GetSenderDID() != sc.GetReceiverDID() {
			td := &model.TransactionDetails{
				TransactionID:   b.GetTid(),
				TransactionType: b.GetTransType(),
				BlockID:         bid,
				Mode:            wallet.FTTransferMode,
				Amount:          sc.GetTotalRBTs(),
				SenderDID:       sc.GetSenderDID(),
				ReceiverDID:     sc.GetReceiverDID(),
				Comment:         sc.GetComment(),
				DateTime:        time.Now(),
				Status:          true,
				Epoch:           int64(transactionEpoch),
			}
			if td.Epoch == 0 {
				td.Epoch = time.Now().Unix()
			}
			c.w.AddTransactionHistory(td)
		}
	}

	// In cvr-2, check pins and state-pins on trans-fts, remove last cvr-1 block from token chain,
	// update token chain with cvr-2 block and update token-status in table
	if cvrStage == wallet.CVRStage2_Sender_to_Receiver {
		results := make([]MultiPinCheckRes, len(tokenInfo))
		var wg sync.WaitGroup
		for i, ti := range tokenInfo {
			t := ti.Token
			c.log.Debug("************* removing cvr-1 block, token : ", t, " token type ", ti.TokenType)

			// if latest block is cvr-1 block, remove it before adding cvr-2 block
			err := c.RemoveSpendableRBTTransferredBlock(t, ti.TokenType)
			if err != nil {
				// TODO : if failed to remove cvr-1 block then store the info in DB and retry it
				c.log.Error(err.Error())
				return nil, nil, fmt.Errorf(err.Error())
			}

			wg.Add(1)
			go c.pinCheck(t, i, senderPeerId, receiverPeerId, results, &wg)
		}
		wg.Wait()
		for i := range results {
			if results[i].Error != nil {
				return nil, nil, fmt.Errorf("Error while checking Token multiple Pins for token %v, error : %v", results[i].Token, results[i].Error)
			}
			if results[i].Status {
				return nil, nil, fmt.Errorf("Token %v has multiple owners: %v", results[i].Token, results[i].Owners)
			}
		}

		tokenStateCheckResult := make([]TokenStateCheckResult, len(tokenInfo))
		for i, ti := range tokenInfo {
			t := ti.Token
			wg.Add(1)
			go c.checkTokenState(t, receiverDID, i, tokenStateCheckResult, &wg, quorumList, ti.TokenType)
		}
		wg.Wait()
		for i := range tokenStateCheckResult {
			if tokenStateCheckResult[i].Error != nil {
				return nil, nil, fmt.Errorf("Error while checking Token State Message : %v", tokenStateCheckResult[i].Message)
			}
			if tokenStateCheckResult[i].Exhausted {
				c.log.Debug("Token state has been exhausted, Token being Double spent:", tokenStateCheckResult[i].Token)
				return nil, nil, fmt.Errorf("Token state has been exhausted, Token being Double spent: %v, msg: %v", tokenStateCheckResult[i].Token, tokenStateCheckResult[i].Message)
			}
			c.log.Debug("Token", tokenStateCheckResult[i].Token, "Message", tokenStateCheckResult[i].Message)
		}
		var FT wallet.FTToken
		FT.FTName = ftinfo.FTName
		updatedTokenStateHashes, err = c.w.FTTokensReceived(receiverDID, tokenInfo, b, senderPeerId, receiverPeerId, c.ipfs, FT)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to update token status, error: %v", err)
		}

		//Adding quorums to DIDPeerTable of receiver
		for _, qrm := range quorumInfo {
			c.w.AddDIDPeerMap(qrm.DID, qrm.PeerID, *qrm.DIDType)
		}
	}

	updateFTTableErr := c.updateFTTable()
	if updateFTTableErr != nil {
		return nil, nil, fmt.Errorf("Failed to update FT table, error: %v", updateFTTableErr)
	}

	return updatedTokenStateHashes, syncIssueTokens, nil
}

func (c *Core) updateReceiverFTHandle(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var sr SendTokenRequest

	err := c.l.ParseJSON(req, &sr)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	// verifying sender signature
	currentBlk := block.InitBlock(sr.TokenChainBlock, nil, block.NoSignature())

	// Debugging block initialization
	if currentBlk == nil {
		c.log.Error("failed to initialise token chain block")
		crep.Message = "failed to initialise token chain block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	validationRes, err := c.ValidateSender(currentBlk)
	if err != nil {
		errMsg := fmt.Sprintf("sender signature verification failed, sender : %v; err : %v", sr.Address, err)
		c.log.Error(errMsg)
		crep.Message = errMsg
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	if !validationRes.Status {
		errMsg := fmt.Sprintf("sender signature verification failed, sender : %v; err : %v", sr.Address, validationRes.Message)
		c.log.Error(errMsg)
		crep.Message = errMsg
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	receiverAddress := c.peerID + "." + did
	updatedtokenhashes, syncIssueTokens, err := c.updateFTToken(
		sr.Address,
		receiverAddress,
		sr.TokenInfo,
		sr.TokenChainBlock,
		sr.QuorumList,
		sr.QuorumInfo,
		sr.TransactionEpoch,
		&sr.FTInfo,
		sr.CVRStage,
	)
	if err != nil {
		c.log.Error(err.Error())
		crep.Message = err.Error()
		crep.Result = syncIssueTokens
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	crep.Status = true
	crep.Message = "Token received successfully"
	crep.Result = updatedtokenhashes

	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) signatureRequest(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var sr SignatureRequest
	err := c.l.ParseJSON(req, &sr)
	srep := SignatureReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		srep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	dc, ok := c.qc[did]
	if !ok {
		c.log.Error("Failed to setup quorum crypto")
		srep.Message = "Failed to setup quorum crypto"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	b := block.InitBlock(sr.TokenChainBlock, nil, block.NoSignature())
	if b == nil {
		c.log.Error("Failed to do signature, invalid token chain block")
		srep.Message = "Failed to do signature, invalid token chanin block"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	sig, err := b.GetSignature(dc)
	if err != nil {
		c.log.Error("Failed to do signature", "err", err)
		srep.Message = "Failed to do signature, " + err.Error()
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	srep.Signature = sig
	srep.Status = true
	srep.Message = "Signature done"
	return c.l.RenderJSON(req, &srep, http.StatusOK)
}

func (c *Core) updatePledgeToken(req *ensweb.Request) *ensweb.Result {
	c.log.Debug("incoming request for pledge finlaity")
	did := c.l.GetQuerry(req, "did")
	c.log.Debug("DID from query", did)
	var ur UpdatePledgeRequest
	err := c.l.ParseJSON(req, &ur)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	crep = c.UpdatePledgeToken(ur, did)
	delete(c.pd, ur.ConsensusReqId)

	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) UpdatePledgeToken(pledgeFinalityReq UpdatePledgeRequest, quorumDID string) model.BasicResponse {
	c.log.Debug("********************pledge finality req, txn id : ", pledgeFinalityReq.TransactionID, "quorumDID ", quorumDID)
	c.log.Debug("^^^^^^^^ is trans-token block empty : ", pledgeFinalityReq.TokenChainBlock == nil)
	crep := model.BasicResponse{
		Status: false,
	}
	dc, ok := c.qc[quorumDID]
	if !ok {
		// c.log.Debug("did crypto initilisation failed")
		c.log.Error("Failed to setup quorum crypto")
		crep.Message = "Failed to setup quorum crypto"
		return crep
	}

	if pledgeFinalityReq.TokenChainBlock == nil {
		errMsg := fmt.Sprintf("tokenchain block bytes are empty, quorum : %v", quorumDID)
		c.log.Error(errMsg)
		crep.Message = errMsg
		return crep
	}
	b := block.InitBlock(pledgeFinalityReq.TokenChainBlock, nil, block.NoSignature())
	if b == nil {
		errMsg := fmt.Sprintf("tokenchain block is empty, quorum: %v", quorumDID)
		c.log.Error(errMsg)
		crep.Message = errMsg
		return crep
	}
	tks := b.GetTransTokens()

	refID := ""
	var refIDArr []string = make([]string, 0)
	if len(tks) > 0 {
		for _, tkn := range tks {
			id, err := b.GetBlockID(tkn)
			if err != nil {
				c.log.Error("Failed to get block ID")
				crep.Message = "Failed to get block ID"
				return crep
			}
			refIDArr = append(refIDArr, fmt.Sprintf("%v_%v_%v", tkn, b.GetTokenType(tkn), id))
		}
	}
	refID = strings.Join(refIDArr, ",")

	ctcb := make(map[string]*block.Block)
	tsb := make([]block.TransTokens, 0)
	// Generally, addition of a token block happens on Sender, Receiver
	// and Quorum's end.
	//
	// If both sender and receiver happen to be on a Non-Quorum server, this is
	// not an issue since we skip TokensTable and Token chain update, if the reciever
	// and sender peer as seem. Thus, multiple update of same block to the Token's tokenchain
	// is avoided
	//
	// However in case either sender or receiver happen to be a Quorum server, even though the above
	// scenario is covered , but since the token block is also added on Quorum's end, we end up in a
	// situation where update of same block happens twice. Hence the following check ensures that we
	// skip the addition of block here, if either sender or receiver happen to be on a Quorum node.
	if !c.w.IsDIDExist(b.GetReceiverDID()) && !c.w.IsDIDExist(b.GetSenderDID()) {
		for _, t := range tks {
			err := c.w.AddTokenBlock(t, b)
			if err != nil {
				c.log.Error("Failed to add token block", "token", t)
				crep.Message = "Failed to add token block"
				return crep
			}
		}
	}
	for _, t := range pledgeFinalityReq.PledgedTokens {
		tk, err := c.w.ReadToken(t)
		if err != nil {
			c.log.Error("failed to read token from wallet")
			crep.Message = "failed to read token from wallet"
			return crep
		}
		ts := RBTString
		if tk.TokenValue != 1.0 {
			ts = PartString
		}
		tt := block.TransTokens{
			Token:     t,
			TokenType: c.TokenType(ts),
		}
		tsb = append(tsb, tt)
		lb := c.w.GetLatestTokenBlock(t, c.TokenType(ts))
		if lb == nil {
			c.log.Error("Failed to get token chain block")
			crep.Message = "Failed to get token chain block"
			return crep
		}
		ctcb[t] = lb
	}

	tcb := block.TokenChainBlock{
		TransactionType: block.TokenPledgedType,
		TokenOwner:      quorumDID,
		TransInfo: &block.TransInfo{
			Comment: "Token is pledged at " + time.Now().String(),
			RefID:   refID,
			Tokens:  tsb,
		},
		Epoch: pledgeFinalityReq.TransactionEpoch,
	}

	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block - qrm rec")
		crep.Message = "Failed to create new token chain block -qrm rec"
		return crep
	}
	err := nb.UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to update signature to block", "err", err)
		crep.Message = "Failed to update signature to block"
		return crep
	}
	err = c.w.CreateTokenBlock(nb)
	if err != nil {
		c.log.Error("Failed to update token chain block", "err", err)
		crep.Message = "Failed to update token chain block"
		return crep
	}
	for _, t := range pledgeFinalityReq.PledgedTokens {
		err = c.w.PledgeWholeToken(quorumDID, t, nb)
		if err != nil {
			c.log.Error("Failed to update pledge token", "err", err)
			crep.Message = "Failed to update pledge token"
			return crep
		}
	}

	//Adding to the Token State Hash Table
	if pledgeFinalityReq.TransferredTokenStateHashes != nil {
		err = c.w.AddTokenStateHash(quorumDID, pledgeFinalityReq.TransferredTokenStateHashes, pledgeFinalityReq.PledgedTokens, pledgeFinalityReq.TransactionID)
		if err != nil {
			c.log.Error("Failed to add token state hash", "err", err)
		}
	}

	crep.Status = true
	crep.Message = "Token pledge status updated"
	return crep
}

func (c *Core) quorumCredit(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var credit CreditScore
	err := c.l.ParseJSON(req, &credit)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse request", "err", err)
		crep.Message = "Failed to parse request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	jb, err := json.Marshal(&credit)
	if err != nil {
		c.log.Error("Failed to parse request", "err", err)
		crep.Message = "Failed to parse request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	// TODO: quorumCredit was earlier used to pass QuorumSignature as Credit information
	// to other nodes. While working on Credit Restructing, this function would require changes.
	// Following nil input to third argument is a temp fix, since quorumCredit is not called anywhere
	// in this implementation
	err = c.w.StoreCredit(did, base64.StdEncoding.EncodeToString(jb), nil)
	if err != nil {
		c.log.Error("Failed to store credit", "err", err)
		crep.Message = "Failed to store credit"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	crep.Status = true
	crep.Message = "Credit accepted"
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) mapDIDArbitration(req *ensweb.Request) *ensweb.Result {
	var m map[string]string
	err := c.l.ParseJSON(req, &m)
	br := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		br.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	od, ok := m["olddid"]
	if !ok {
		c.log.Error("Missing old did value")
		br.Message = "Missing old did value"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	nd, ok := m["newdid"]
	if !ok {
		c.log.Error("Missing new did value")
		br.Message = "Missing new did value"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	err = c.srv.UpdateTokenDetials(nd)
	if err != nil {
		c.log.Error("Failed to update table details", "err", err)
		br.Message = "Failed to update token details"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	dm := &service.DIDMap{
		OldDID: od,
		NewDID: nd,
	}
	err = c.srv.UpdateDIDMap(dm)
	if err != nil {
		c.log.Error("Failed to update map table", "err", err)
		br.Message = "Failed to update map table"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	br.Status = true
	br.Message = "DID mapped successfully"
	return c.l.RenderJSON(req, &br, http.StatusOK)
}

func (c *Core) chekDIDArbitration(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "olddid")
	br := model.BasicResponse{
		Status: true,
	}
	if c.srv.IsDIDExist(did) {
		br.Message = "DID exist"
		br.Result = true
	} else {
		br.Message = "DID does not exist"
		br.Result = false
	}
	return c.l.RenderJSON(req, &br, http.StatusOK)
}

func (c *Core) getTokenNumber(req *ensweb.Request) *ensweb.Result {
	var hashes []string
	br := model.TokenNumberResponse{
		Status: false,
	}
	err := c.l.ParseJSON(req, &hashes)
	if err != nil {
		br.Message = "failed to get token number, parsing failed"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	tns := make([]int, 0)
	for i := range hashes {
		tn, err := c.srv.GetTokenNumber(hashes[i])
		if err != nil {
			tns = append(tns, -1)
		} else {
			tns = append(tns, tn)
		}
	}
	br.Status = true
	br.TokenNumbers = tns
	return c.l.RenderJSON(req, &br, http.StatusOK)
}

func (c *Core) getMigratedTokenStatus(req *ensweb.Request) *ensweb.Result {
	var tokens []string
	br := model.MigratedTokenStatus{
		Status: false,
	}
	err := c.l.ParseJSON(req, &tokens)
	if err != nil {
		br.Message = "failed to get tokens, parsing failed"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	migratedTokenStatus := make([]int, 0)
	for i := range tokens {
		td, err := c.srv.GetTokenDetials(tokens[i])
		if err == nil && td.Token == tokens[i] {
			migratedTokenStatus[i] = 1
		}
	}
	br.Status = true
	br.MigratedStatus = migratedTokenStatus
	return c.l.RenderJSON(req, &br, http.StatusOK)
}

func (c *Core) syncDIDArbitration(req *ensweb.Request) *ensweb.Result {
	var m map[string]string
	err := c.l.ParseJSON(req, &m)
	br := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		br.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	od, ok := m["olddid"]
	if !ok {
		c.log.Error("Missing old did value")
		br.Message = "Missing old did value"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	nd, ok := m["newdid"]
	if !ok {
		c.log.Error("Missing new did value")
		br.Message = "Missing new did value"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	dm := &service.DIDMap{
		OldDID: od,
		NewDID: nd,
	}
	err = c.srv.UpdateDIDMap(dm)
	if err != nil {
		c.log.Error("Failed to update map table", "err", err)
		br.Message = "Failed to update map table"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	br.Status = true
	br.Message = "DID mapped successfully"
	return c.l.RenderJSON(req, &br, http.StatusOK)

}

func (c *Core) tokenArbitration(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var sr SignatureRequest
	err := c.l.ParseJSON(req, &sr)
	srep := SignatureReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		srep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}

	b := block.InitBlock(sr.TokenChainBlock, nil, block.NoSignature())
	if b == nil {
		c.log.Error("Failed to do token abitration, invalid token chain block")
		srep.Message = "Failed to do token abitration, invalid token chanin block"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	scb := b.GetSmartContract()
	if scb == nil {
		c.log.Error("Failed to do token abitration, invalid token chain block, missing smart contract")
		srep.Message = "Failed to do token abitration, invalid token chain block, missing smart contract"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	sc := contract.InitContract(scb, nil)
	if sc == nil {
		c.log.Error("Failed to do token abitration, invalid smart contract")
		srep.Message = "Failed to do token abitration, invalid smart contract"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	ti := sc.GetTransTokenInfo()
	if ti == nil {
		c.log.Error("Failed to do token abitration, invalid token")
		srep.Message = "Failed to do token abitration, invalid token"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	mflag := false
	mmsg := "token is already migrated"
	for i := range ti {
		tl, tn, err := b.GetTokenDetials(ti[i].Token)
		if err != nil {
			c.log.Error("Failed to do token abitration, invalid token detials", "err", err)
			srep.Message = "Failed to do token abitration, invalid token detials"
			return c.l.RenderJSON(req, &srep, http.StatusOK)
		}
		str := token.GetTokenString(tl, tn)
		tbr := bytes.NewBuffer([]byte(str))
		thash, err := c.ipfs.Add(tbr, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		if err != nil {
			c.log.Error("Failed to do token abitration, failed to get ipfs hash", "err", err)
			srep.Message = "Failed to do token abitration, failed to get ipfs hash"
			return c.l.RenderJSON(req, &srep, http.StatusOK)
		}
		if thash != ti[i].Token {
			c.log.Error("Failed to do token abitration, token hash not matching", "thash", thash, "token", ti[i].Token)
			srep.Message = "Failed to do token abitration, token hash not matching"
			return c.l.RenderJSON(req, &srep, http.StatusOK)
		}

		odid := ti[i].OwnerDID
		if odid == "" {
			c.log.Error("Failed to do token abitration, invalid owner did")
			srep.Message = "Failed to do token abitration, invalid owner did"
			return c.l.RenderJSON(req, &srep, http.StatusOK)
		}
		td, err := c.srv.GetTokenDetials(ti[i].Token)
		if err == nil && td.Token == ti[i].Token {
			nm, _ := c.srv.GetNewDIDMap(td.DID)
			c.log.Error("Failed to do token abitration, token is already migrated", "token", ti[i].Token, "old_did", nm.OldDID, "new_did", td.DID)
			mflag = true
			mmsg = mmsg + "," + ti[i].Token
			// srep.Message = "token is already migrated," + ti[i].Token
			// return c.l.RenderJSON(req, &srep, http.StatusOK)
		}
		if !mflag {
			dc, err := c.SetupForienDID(odid, "")
			if err != nil {
				c.log.Error("Failed to do token abitration, failed to setup did crypto", "token", ti[i].Token, "did", odid)
				srep.Message = "Failed to do token abitration, failed to setup did crypto"
				return c.l.RenderJSON(req, &srep, http.StatusOK)
			}
			err = sc.VerifySignature(dc)
			if err != nil {
				c.log.Error("Failed to do token abitration, signature verification failed", "err", err)
				srep.Message = "Failed to do token abitration, signature verification failed"
				return c.l.RenderJSON(req, &srep, http.StatusOK)
			}
			err = c.srv.UpdateTempTokenDetials(&service.TokenDetials{Token: ti[i].Token, DID: odid})
			if err != nil {
				c.log.Error("Failed to do token abitration, failed update token detials", "err", err)
				srep.Message = "Failed to do token abitration, failed update token detials"
				return c.l.RenderJSON(req, &srep, http.StatusOK)
			}
		}
	}
	if mflag {
		srep.Message = mmsg
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	dc, ok := c.qc[did]
	if !ok {
		c.log.Error("Failed to setup quorum crypto")
		srep.Message = "Failed to setup quorum crypto"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	sig, err := b.GetSignature(dc)
	if err != nil {
		c.log.Error("Failed to do token abitration, failed to get signature", "err", err)
		srep.Message = "Failed to do token abitration, failed to get signature"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	srep.Signature = sig
	srep.Status = true
	srep.Message = "Signature done"
	return c.l.RenderJSON(req, &srep, http.StatusOK)
}

func (c *Core) unlockTokens(req *ensweb.Request) *ensweb.Result {
	var tokenList TokenList
	err := c.l.ParseJSON(req, &tokenList)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = c.w.UnlockLockedTokens(tokenList.DID, tokenList.Tokens, c.testNet)
	if err != nil {
		c.log.Error("Failed to update token status", "err", err)
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	crep.Status = true
	crep.Message = "Tokens Unlocked Successfully."
	c.log.Info("Tokens Unlocked")
	return c.l.RenderJSON(req, &crep, http.StatusOK)

}

func (c *Core) updateTokenHashDetails(req *ensweb.Request) *ensweb.Result {
	c.log.Debug("Updating tokenStateHashDetails in DB")
	tokenIDTokenStateHash := c.l.GetQuerry(req, "tokenIDTokenStateHash")
	c.log.Debug("tokenIDTokenStateHash from query", tokenIDTokenStateHash)

	err := c.w.RemoveTokenStateHash(tokenIDTokenStateHash)
	if err == nil {
		fmt.Println("removed hash successfully")
	}
	return c.l.RenderJSON(req, struct{}{}, http.StatusOK)

}

func (c *Core) notifyUnusedQuorumsResponse(req *ensweb.Request) *ensweb.Result {
	// unused quorums are notified to unlock pledge tokens by API : /api/notify-unused-quorums
	resp := &model.BasicResponse{
		Status: false,
	}
	var consensusRequestID string
	err := c.l.ParseJSON(req, &consensusRequestID)
	c.log.Debug("notification to unlock locked tokens to pledge")
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse json request, err : %v", err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return c.l.RenderJSON(req, resp, http.StatusOK)
	}

	// fetch locked tokens details and unlock
	pd, ok := c.pd[consensusRequestID]
	// if !ok {
	// 	errMsg := fmt.Sprintf("invalid pledge tokens details for consensus req id : %v", consensusRequestID)
	// 	c.log.Error(errMsg)
	// 	resp.Message = errMsg
	// 	return c.l.RenderJSON(req, resp, http.StatusOK)
	// }
	if ok {
		// unlock all the locked tokens to pledge for the given consensus request ID
		for did, lockedTokens := range pd.PledgedTokens {
			err = c.w.UnlockLockedTokens(did, lockedTokens, c.testNet)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to update token status, err : %v", err)
				c.log.Error(errMsg)
				resp.Message = errMsg
				return c.l.RenderJSON(req, resp, http.StatusOK)
			}
		}

		// delete the map from core struct
		delete(c.pd, consensusRequestID)
	}

	resp.Status = true
	resp.Message = "unlocked pledge tokens successfully"
	return c.l.RenderJSON(req, resp, http.StatusOK)
}

type ReceiverRollBack struct {
	TokenStatusMap map[int][]string `json:"token_status_map"`
	TxnID          string           `json:"txn_id"`
	TokenType      string           `json:"token_type"`
}

func (c *Core) notifyReceiverToRollback(req *ensweb.Request) *ensweb.Result {
	// unused quorums are notified to unlock pledge tokens by API : /api/notify-unused-quorums
	resp := &model.BasicResponse{
		Status: false,
	}
	var receiverRollBackReq ReceiverRollBack
	err := c.l.ParseJSON(req, &receiverRollBackReq)
	c.log.Debug("notification to update the received tokens to requested status")
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse json request, err : %v", err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return c.l.RenderJSON(req, resp, http.StatusOK)
	}

	rollBackFailedTokens := make([]string, 0)

	resp.Status = true
	// read token and update status and chain as per token type
	switch receiverRollBackReq.TokenType {
	case RBTString, PartString:
		for tokenStatus, tokensList := range receiverRollBackReq.TokenStatusMap {
			for _, tokenID := range tokensList {
				rbtInfo, err := c.w.ReadToken(tokenID)
				if err != nil {
					errMsg := fmt.Sprintf("failed to read RBT info from Tokens Table to rollback; tokenId : %v, err : %v", tokenID, err)
					c.log.Error(errMsg)
					rollBackFailedTokens = append(rollBackFailedTokens, tokenID)
					resp.Status = resp.Status && false
					continue
				}

				// get token type string
				tokenTypeStr := RBTString
				if rbtInfo.TokenValue != 1 {
					tokenTypeStr = PartString
				}

				// if the provided token has the provided txn id and is a free token then only update the token info
				if rbtInfo.TokenStatus == wallet.TokenIsFree && rbtInfo.TransactionID == receiverRollBackReq.TxnID {
					rbtInfo.TokenStatus = tokenStatus
					err = c.w.UpdateToken(rbtInfo)
					if err != nil {
						errMsg := fmt.Sprintf("failed to update RBT status in tokens table while rollback; tokenId : %v, err : %v", tokenID, err)
						c.log.Error(errMsg)
						rollBackFailedTokens = append(rollBackFailedTokens, tokenID)
						resp.Status = resp.Status && false
						continue
					}
					// remove last cvr-1 block
					err = c.RemoveSpendableRBTTransferredBlock(tokenID, c.TokenType(tokenTypeStr))
					if err != nil {
						errMsg := fmt.Sprintf("failed to remove latest block of RBT chain while rollback; tokenId : %v, err : %v", tokenID, err)
						c.log.Error(errMsg)
						rollBackFailedTokens = append(rollBackFailedTokens, tokenID)
						resp.Status = resp.Status && false
						continue
					}
				}
			}
		}
	case FTString:
		for tokenStatus, tokensList := range receiverRollBackReq.TokenStatusMap {
			for _, tokenID := range tokensList {
				ftInfo, err := c.w.ReadFTToken(tokenID)
				if err != nil {
					errMsg := fmt.Sprintf("failed to read FT info from FtTable to rollback; tokenId : %v, err : %v", tokenID, err)
					c.log.Error(errMsg)
					rollBackFailedTokens = append(rollBackFailedTokens, tokenID)
					resp.Status = resp.Status && false
					continue
				}
				// if the provided token has the provided txn id and is a free token then only update the token info
				if ftInfo.TokenStatus == wallet.TokenIsFree && ftInfo.TransactionID == receiverRollBackReq.TxnID {
					ftInfo.TokenStatus = tokenStatus
					err = c.w.UpdateFTToken(ftInfo)
					if err != nil {
						errMsg := fmt.Sprintf("failed to update FT status in FT table while rollback; tokenId : %v, err : %v", tokenID, err)
						c.log.Error(errMsg)
						rollBackFailedTokens = append(rollBackFailedTokens, tokenID)
						resp.Status = resp.Status && false
						continue
					}
					// remove last cvr-1 block
					err = c.RemoveSpendableRBTTransferredBlock(tokenID, token.FTTokenType)
					if err != nil {
						errMsg := fmt.Sprintf("failed to remove latest block of FT chain while rollback; tokenId : %v, err : %v", tokenID, err)
						c.log.Error(errMsg)
						rollBackFailedTokens = append(rollBackFailedTokens, tokenID)
						resp.Status = resp.Status && false
						continue
					}
				}
			}
		}
	default:
		// TODO : handle double spent issue for NFT
		errMsg := fmt.Sprintf("invalid token type : %v, cannot rollback directly", receiverRollBackReq.TokenType)
		c.log.Error(errMsg)
		resp.Status = false
		resp.Message = errMsg
		return c.l.RenderJSON(req, resp, http.StatusOK)
	}

	if !resp.Status {
		errMsg := fmt.Sprintf("receiver's roll back failed for %v tokens", len(rollBackFailedTokens))
		resp.Message = errMsg
		resp.Result = rollBackFailedTokens
	} else {
		resp.Status = true
		resp.Message = "receiver rolled backed double-spent tokens successfully"
	}
	return c.l.RenderJSON(req, resp, http.StatusOK)
}
