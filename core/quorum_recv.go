package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/service"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	didcrypto "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

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

func (c *Core) verifyContract(cr *ConensusRequest) (bool, *contract.Contract) {
	sc := contract.InitContract(cr.ContractBlock, nil)
	// setup the did to verify the signature
	dc, err := c.SetupForienDID(sc.GetSenderDID())
	if err != nil {
		c.log.Error("Failed to get DID", "err", err)
		return false, nil
	}
	err = sc.VerifySignature(dc)
	if err != nil {
		c.log.Error("Failed to verify sender signature", "err", err)
		return false, nil
	}
	return true, sc
}

func (c *Core) quorumDTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	ok, sc := c.verifyContract(cr)
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
	ok, sc := c.verifyContract(cr)
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
		go c.pinCheck(ti[i].Token, i, cr.SenderPeerID, cr.ReceiverPeerID, results, &wg)
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

	validateTokenOwnershipVar, err := c.validateTokenOwnership(cr, sc, did)
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
	receiver := cr.ReceiverPeerID + "." + sc.GetReceiverDID()
	err1 := c.pinTokenState(tokenStateCheckResult, did, cr.TransactionID, sender, receiver, float64(0))
	if err1 != nil {
		crep.Message = "Error Pinning token state" + err.Error()
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	c.log.Debug("Finished Tokenstate check")

	//check if token is pledgedtoken
	wt := sc.GetTransTokenInfo()

	for i := range wt {
		b := c.w.GetLatestTokenBlock(wt[i].Token, wt[i].TokenType)
		if b == nil {
			c.log.Error("pledge token check Failed, failed to get latest block")
			crep.Message = "pledge token check Failed, failed to get latest block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if c.checkIsPledged(b) {
			c.log.Error("Pledge Token check Failed, Token ", wt[i], " is Pledged Token")
			crep.Message = "Pledge Token check Failed, Token " + wt[i].Token + " is Pledged Token"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if c.checkIsUnpledged(b) {
			unpledgeId := c.getUnpledgeId(wt[i].Token)
			if unpledgeId == "" {
				c.log.Error("Failed to fetch proof file CID")
				crep.Message = "Failed to fetch proof file CID"
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			err := c.ipfs.Get(unpledgeId, c.cfg.DirPath+"unpledge")
			if err != nil {
				c.log.Error("Failed to fetch proof file")
				crep.Message = "Failed to fetch proof file, err " + err.Error()
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			pcb, err := ioutil.ReadFile(c.cfg.DirPath + "unpledge/" + unpledgeId)
			if err != nil {
				c.log.Error("Invalid file", "err", err)
				crep.Message = "Invalid file,err " + err.Error()
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			pcs := util.BytesToString(pcb)

			senderAddr := cr.SenderPeerID + "." + sc.GetSenderDID()
			rdid, tid, err := c.getProofverificationDetails(wt[i].Token, senderAddr)
			if err != nil {
				c.log.Error("Failed to get pledged for token reciveer did", "err", err)
				crep.Message = "Failed to get pledged for token reciveer did"
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			pv, err := c.up.ProofVerification(wt[i].Token, pcs, rdid, tid)
			if err != nil {
				c.log.Error("Proof Verification Failed due to error ", err)
				crep.Message = "Proof Verification Failed due to error " + err.Error()
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			if !pv {
				c.log.Debug("Proof of Work for Unpledge not verified")
				crep.Message = "Proof of Work for Unpledge not verified"
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			c.log.Debug("Proof of work verified")
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

func (c *Core) quorumNFTSaleConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	ok, sc := c.verifyContract(cr)
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
	validateTokenOwnershipVar, err := c.validateTokenOwnership(cr, sc, did)
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

	dc, err := c.SetupForienDID(verifyDID)
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

	//check if deployment or execution

	var tokenStateCheckResult []TokenStateCheckResult
	var wg sync.WaitGroup
	if consensusRequest.Mode == SmartContractDeployMode {
		//if deployment
		commitedTokenInfo := consensusContract.GetCommitedTokensInfo()
		//1. check commited token authenticity
		c.log.Debug("validation 1 - Authenticity of commited RBT tokens")
		validateTokenOwnershipVar, err := c.validateTokenOwnership(consensusRequest, consensusContract, did)
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

	qHash := util.CalculateHash(consensusContract.GetBlock(), "SHA3-256")
	qsb, ppb, err := qdc.Sign(util.HexToStr(qHash))
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		consensusReply.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
	}

	consensusReply.Status = true
	consensusReply.Message = "Consensus finished successfully"
	consensusReply.ShareSig = qsb
	consensusReply.PrivSig = ppb
	return c.l.RenderJSON(req, &consensusReply, http.StatusOK)
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
	case RBTTransferMode:
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
	c.log.Debug("Request for pledge")
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	if (pr.TokensRequired) < MinTrnxAmt {
		c.log.Error("Pledge amount is less than ", MinTrnxAmt)
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
	wt, err := c.GetTokens(dc, did, pr.TokensRequired)
	if err != nil {
		crep.Message = "Failed to get tokens"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	tl := len(wt)
	if tl == 0 {
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

	for i := 0; i < tl; i++ {
		presp.Tokens = append(presp.Tokens, wt[i].TokenID)
		presp.TokenValue = append(presp.TokenValue, wt[i].TokenValue)
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
	return c.l.RenderJSON(req, &presp, http.StatusOK)
}

func (c *Core) updateReceiverToken(req *ensweb.Request) *ensweb.Result {
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
	b := block.InitBlock(sr.TokenChainBlock, nil)
	if b == nil {
		c.log.Error("Invalid token chain block", "err", err)
		crep.Message = "Invalid token chain block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	p, err := c.getPeer(sr.Address)
	if err != nil {
		c.log.Error("failed to get peer", "err", err)
		crep.Message = "failed to get peer"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	defer p.Close()
	for _, ti := range sr.TokenInfo {
		t := ti.Token
		pblkID, err := b.GetPrevBlockID(t)
		if err != nil {
			c.log.Error("failed to sync token chain block, missing previous block id for token ", t, " err : ", err)
			crep.Message = "failed to sync token chain block, missing previous block id for token " + t
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		err = c.syncTokenChainFrom(p, pblkID, t, ti.TokenType)
		if err != nil {
			errMsg := fmt.Sprintf("failed to sync tokenchain Token: %v, issueType: %v", t, TokenChainNotSynced)
			c.log.Error(errMsg, "err", err)
			crep.Message = errMsg
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}

		if c.TokenType(PartString) == ti.TokenType {
			gb := c.w.GetGenesisTokenBlock(t, ti.TokenType)
			if gb == nil {
				c.log.Error("failed to get genesis block for token ", t, "err : ", err)
				crep.Message = "failed to get genesis block for token " + t
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			pt, _, err := gb.GetParentDetials(t)
			if err != nil {
				c.log.Error("failed to get parent details for token ", t, " err : ", err)
				crep.Message = "failed to get parent details for token " + t
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
			err = c.syncParentToken(p, pt)
			if err != nil {
				c.log.Error("failed to sync parent token ", pt, " childtoken ", t, " err : ", err)
				crep.Message = "failed to sync parent token " + pt + " childtoken " + t
				return c.l.RenderJSON(req, &crep, http.StatusOK)
			}
		}
		ptcbArray, err := c.w.GetTokenBlock(t, ti.TokenType, pblkID)
		if err != nil {
			c.log.Error("Failed to fetch previous block for token ", t, " err : ", err)
			crep.Message = "Failed to fetch previous block for token " + t
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		ptcb := block.InitBlock(ptcbArray, nil)
		if c.checkIsPledged(ptcb) {
			c.log.Error("Token is a pledged Token", "token", t)
			crep.Message = "Token " + t + " is a pledged Token"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}

	results := make([]MultiPinCheckRes, len(sr.TokenInfo))
	var wg sync.WaitGroup
	for i, ti := range sr.TokenInfo {
		t := ti.Token
		senderPeerId, _, ok := util.ParseAddress(sr.Address)
		if !ok {
			c.log.Error("Error occured", "error", err)
			crep.Message = "Unable to parse sender address"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		wg.Add(1)
		go c.pinCheck(t, i, senderPeerId, c.peerID, results, &wg)
	}
	wg.Wait()
	for i := range results {
		if results[i].Error != nil {
			c.log.Error("Error occured", "error", err)
			crep.Message = "Error while cheking Token multiple Pins"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if results[i].Status {
			c.log.Error("Token has multiple owners", "token", results[i].Token, "owners", results[i].Owners)
			crep.Message = "Token has multiple owners"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}

	//tokenstate check

	tokenStateCheckResult := make([]TokenStateCheckResult, len(sr.TokenInfo))
	for i, ti := range sr.TokenInfo {
		t := ti.Token
		wg.Add(1)
		go c.checkTokenState(t, did, i, tokenStateCheckResult, &wg, sr.QuorumList, ti.TokenType)
	}
	wg.Wait()

	for i := range tokenStateCheckResult {
		if tokenStateCheckResult[i].Error != nil {
			c.log.Error("Error occured", "error", err)
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
	senderPeerId, _, _ := util.ParseAddress(sr.Address)
	err = c.w.TokensReceived(did, sr.TokenInfo, b, senderPeerId, c.peerID)
	if err != nil {
		c.log.Error("Failed to update token status", "err", err)
		crep.Message = "Failed to update token status"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	sc := contract.InitContract(b.GetSmartContract(), nil)
	if sc == nil {
		c.log.Error("Failed to update token status, missing smart contract")
		crep.Message = "Failed to update token status, missing smart contract"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	bid, err := b.GetBlockID(sr.TokenInfo[0].Token)
	if err != nil {
		c.log.Error("Failed to update token status, failed to get block ID", "err", err)
		crep.Message = "Failed to update token status, failed to get block ID"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	td := &wallet.TransactionDetails{
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
	}
	c.w.AddTransactionHistory(td)
	crep.Status = true
	crep.Message = "Token received successfully"
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
	dc, ok := c.qc[did]
	if !ok {
		c.log.Debug("did crypto initilisation failed")
		c.log.Error("Failed to setup quorum crypto")
		crep.Message = "Failed to setup quorum crypto"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	b := block.InitBlock(ur.TokenChainBlock, nil)
	tks := b.GetTransTokens()
	refID := ""
	if len(tks) > 0 {
		id, err := b.GetBlockID(tks[0])
		if err != nil {
			c.log.Error("Failed to get block ID")
			crep.Message = "Failed to get block ID"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		refID = fmt.Sprintf("%s,%d,%s", tks[0], b.GetTokenType(tks[0]), id)
	}

	ctcb := make(map[string]*block.Block)
	tsb := make([]block.TransTokens, 0)
	for _, t := range tks {
		err = c.w.AddTokenBlock(t, b)
		if err != nil {
			c.log.Error("Failed to add token block", "token", t)
			crep.Message = "Failed to add token block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}
	for _, t := range ur.PledgedTokens {
		tk, err := c.w.ReadToken(t)
		if err != nil {
			c.log.Error("failed to read token from wallet")
			crep.Message = "failed to read token from wallet"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
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
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		ctcb[t] = lb
	}
	tcb := block.TokenChainBlock{
		TransactionType: block.TokenPledgedType,
		TokenOwner:      did,
		TransInfo: &block.TransInfo{
			Comment: "Token is pledged at " + time.Now().String(),
			RefID:   refID,
			Tokens:  tsb,
		},
	}
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block - qrm rec")
		crep.Message = "Failed to create new token chain block -qrm rec"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = nb.UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to update signature to block", "err", err)
		crep.Message = "Failed to update signature to block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = c.w.CreateTokenBlock(nb)
	if err != nil {
		c.log.Error("Failed to update token chain block", "err", err)
		crep.Message = "Failed to update token chain block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	for _, t := range ur.PledgedTokens {
		err = c.w.PledgeWholeToken(did, t, nb)
		if err != nil {
			c.log.Error("Failed to update pledge token", "err", err)
			crep.Message = "Failed to update pledge token"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}

	for _, t := range ur.PledgedTokens {
		c.up.AddUnPledge(t)
	}
	crep.Status = true
	crep.Message = "Token pledge status updated"
	return c.l.RenderJSON(req, &crep, http.StatusOK)
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
	err = c.w.StoreCredit(did, base64.StdEncoding.EncodeToString(jb))
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
		c.log.Error("Failed to update table detials", "err", err)
		br.Message = "Failed to update token detials"
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
			dc, err := c.SetupForienDID(odid)
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

func (c *Core) getProofverificationDetails(tokenID string, senderAddr string) (string, string, error) {
	var receiverDID, txnId string
	tt := token.RBTTokenType
	blk := c.w.GetLatestTokenBlock(tokenID, tt)

	pbid, err := blk.GetPrevBlockID(tokenID)
	if err != nil {
		c.log.Error("Failed to get the block id. Unable to verify proof file")
		return "", "", err
	}
	pBlk, err := c.w.GetTokenBlock(tokenID, tt, pbid)
	if err != nil {
		c.log.Error("Failed to get the Previous Block Unable to verify proof file")
		return "", "", err
	}

	prevBlk := block.InitBlock(pBlk, nil)
	if prevBlk == nil {
		c.log.Error("Failed to initialize the Previous Block Unable to verify proof file")
		return "", "", fmt.Errorf("Failed to initilaize previous block")
	}
	tokenPledgedForDetailsStr := prevBlk.GetTokenPledgedForDetails()
	tokenPledgedForDetailsBlkArray := prevBlk.GetTransBlock()

	if tokenPledgedForDetailsStr == "" && tokenPledgedForDetailsBlkArray == nil {
		c.log.Error("Failed to get details pledged for token. Unable to verify proof file")
		return "", "", fmt.Errorf("Failed to get deatils of pledged for token")
	}

	if tokenPledgedForDetailsBlkArray != nil {
		tokenPledgedForDetailsBlk := block.InitBlock(tokenPledgedForDetailsBlkArray, nil)
		receiverDID = tokenPledgedForDetailsBlk.GetReceiverDID()
		txnId = tokenPledgedForDetailsBlk.GetTid()
	}

	if tokenPledgedForDetailsStr != "" {
		tpfdArray := strings.Split(tokenPledgedForDetailsStr, ",")

		tokenPledgedFor := tpfdArray[0]
		tokenPledgedForTypeStr := tpfdArray[1]
		tokenPledgedForBlockId := tpfdArray[2]

		tokenPledgedForType, err := strconv.Atoi(tokenPledgedForTypeStr)
		if err != nil {
			c.log.Error("Failed toconvert to integer", "err", err)
			return "", "", err
		}
		//check if token chain of token pledged for already synced to node
		pledgedfBlk, err := c.w.GetTokenBlock(tokenPledgedFor, tokenPledgedForType, tokenPledgedForBlockId)
		if err != nil {
			c.log.Error("Failed to get the pledged for token's Block Unable to verify proof file")
			return "", "", err
		}
		var pledgedforBlk *block.Block
		if pledgedfBlk != nil {
			pledgedforBlk = block.InitBlock(pledgedfBlk, nil)
			if pledgedforBlk == nil {
				c.log.Error("Failed to initialize the pledged for token's Block Unable to verify proof file")
				return "", "", fmt.Errorf("Failed to initilaize previous block")
			}
		} else { //if token chain of token pledged for not synced fetch from sender
			p, err := c.getPeer(senderAddr)
			if err != nil {
				c.log.Error("Failed to get peer", "err", err)
				return "", "", err
			}
			err = c.syncTokenChainFrom(p, tokenPledgedForBlockId, tokenPledgedFor, tokenPledgedForType)
			if err != nil {
				c.log.Error("Failed to sync token chain block", "err", err)
				return "", "", err
			}
			tcbArray, err := c.w.GetTokenBlock(tokenPledgedFor, tokenPledgedForType, tokenPledgedForBlockId)
			if err != nil {
				c.log.Error("Failed to fetch previous block", "err", err)
				return "", "", err
			}
			pledgedforBlk = block.InitBlock(tcbArray, nil)
		}

		receiverDID = pledgedforBlk.GetReceiverDID()
		txnId = pledgedforBlk.GetTid()
	}
	return receiverDID, txnId, nil
}
