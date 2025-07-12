package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIGetAllTokens(req *ensweb.Request) *ensweb.Result {
	tokenType := s.GetQuerry(req, "type")
	did := s.GetQuerry(req, "did")
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	tr, err := s.c.GetAllTokens(did, tokenType)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get tokens", nil)
	}
	return s.RenderJSON(req, tr, http.StatusOK)
}

func (s *Server) APIGenerateTestToken(req *ensweb.Request) *ensweb.Result {
	var tr model.RBTGenerateRequest
	err := s.ParseJSON(req, &tr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(tr.DID)
	if !strings.HasPrefix(tr.DID, "bafybmi") || len(tr.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	if tr.NumberOfTokens <= 0 {
		s.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
		return s.BasicResponse(req, false, "Invalid RBT amount, tokens generated should be a whole number and greater than 0", nil)
	}

	if !s.validateDIDAccess(req, tr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.GenerateTestTokens(req.ID, tr.NumberOfTokens, tr.DID)
	return s.didResponse(req, req.ID)
}

type RBTTransferRequestSwaggoInput struct {
	Receiver   string  `json:"receiver"`
	Sender     string  `json:"sender"`
	TokenCount float64 `json:"tokenCOunt"`
	Comment    string  `json:"comment"`
	Type       int     `json:"type"`
}

// ShowAccount godoc

// @Summary     Initiate RBT Transfer
// @Description This API will initiate RBT transfer to the specified dID
// @Tags        Account
// @ID 			initiate-rbt-transfer
// @Accept      json
// @Produce     json
// @Param 		input body RBTTransferRequestSwaggoInput true "Intitate RBT transfer"
// @Success 200 {object} model.BasicResponse
// @Router /api/initiate-rbt-transfer [post]
func (s *Server) APIInitiateRBTTransfer(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTTransferRequest
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, senderDID, ok := util.ParseAddress(rbtReq.Sender)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	rbtReq.Sender = senderDID
	_, reciverDID, ok := util.ParseAddress(rbtReq.Receiver)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	rbtReq.Receiver = reciverDID

	is_alphanumeric_sender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(rbtReq.Sender)
	is_alphanumeric_receiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(rbtReq.Receiver)
	if !is_alphanumeric_sender || !is_alphanumeric_receiver {
		s.log.Error("Invalid sender or receiver address. Please provide valid DID")
		return s.BasicResponse(req, false, "Invalid sender or receiver address", nil)
	}
	if !strings.HasPrefix(rbtReq.Sender, "bafybmi") || len(rbtReq.Sender) != 59 || !strings.HasPrefix(rbtReq.Receiver, "bafybmi") || len(rbtReq.Receiver) != 59 {
		s.log.Error("Invalid sender or receiver DID")
		return s.BasicResponse(req, false, "Invalid sender or receiver DID", nil)
	}
	if rbtReq.TokenCount < 0.001 {
		s.log.Error("Invalid RBT amount. RBT amount should be atlease 0.001")
		return s.BasicResponse(req, false, "Invalid RBT amount. RBT amount should be atlease 0.001", nil)
	}
	if rbtReq.Type < 1 || rbtReq.Type > 2 {
		s.log.Error("Invalid trans type. TransType should be 1 or 2")
		return s.BasicResponse(req, false, "Invalid trans type. TransType should be 1 or 2", nil)
	}
	if !s.validateDIDAccess(req, rbtReq.Sender) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateRBTTransfer(req.ID, &rbtReq)
	return s.didResponse(req, req.ID)
}

// function for Pinning RBT as service

type RBTPinRequestSwaggoInput struct {
	PinningNode string  `json:"pinningNode"`
	Sender      string  `json:"sender"`
	TokenCount  float64 `json:"tokenCOunt"`
	Comment     string  `json:"comment"`
	Type        int     `json:"type"`
}

// ShowAccount godoc
// @Summary     Initiate Pin Token
// @Description This API will pin token in the Pinning node on behalf of the sender
// @Tags        Account
// @ID 			initiate-pin-token
// @Accept      json
// @Produce     json
// @Param 		input body RBTPinRequestSwaggoInput true "Intitate Pin Token"
// @Success 200 {object} model.BasicResponse
// @Router /api/initiate-pin-token [post]
func (s *Server) APIInitiatePinRBT(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTPinRequest
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(rbtReq.Sender)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiatePinRBT(req.ID, &rbtReq)
	return s.didResponse(req, req.ID)
}

type RBTRecoverRequestSwaggoInput struct {
	PinningNode string  `json:"pinningNode"`
	Sender      string  `json:"sender"`
	TokenCount  float64 `json:"tokenCOunt"`
	Password    string  `json:"password"`
}

// ShowAccount godoc
// @Summary     Recover Token and Tokenchain from the pinning node
// @Description This API will recover token and tokenchain from the Pinning node to the node which has pinned the token
// @Tags        Account
// @ID 			recover-token
// @Accept      json
// @Produce     json
// @Param 		input body RBTRecoverRequestSwaggoInput true "Recover-Token"
// @Success 200 {object} model.BasicResponse
// @Router /api/recover-token [post]
func (s *Server) APIRecoverRBT(req *ensweb.Request) *ensweb.Result {
	var rbtReq model.RBTRecoverRequest
	err := s.ParseJSON(req, &rbtReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(rbtReq.Sender)
	if !ok {
		return s.BasicResponse(req, false, "Invalid sender address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateRecoverRBT(req.ID, &rbtReq)
	return s.didResponse(req, req.ID)
}

// ShowAccount godoc
// @Summary      Check account balance
// @Description  For a mentioned DID, check the account balance
// @Tags         Account
// @Accept       json
// @Produce      json
// @Param        did      	   query      string  true  "User DID"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-account-info [get]
func (s *Server) APIGetAccountInfo(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID:", did)
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	info, err := s.c.GetAccountInfo(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	ac := model.GetAccountInfo{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got account info successfully",
		},
		AccountInfo: make([]model.DIDAccountInfo, 0),
	}
	ac.AccountInfo = append(ac.AccountInfo, info)

	return s.RenderJSON(req, ac, http.StatusOK)
}

type SignatureResponseSwaggoInput struct {
	ID       string `json:"id"`
	Mode     int    `json:"mode"`
	Password string `json:"password"`
}

// ShowAccount godoc
// @Summary     Signature Response
// @Description This API is used to supply the password for the node along with the ID generated when Initiate RBT transfer is called.
// @Tags        Account
// @ID 			signature-response
// @Accept      json
// @Produce     json
// @Param 		input body SignatureResponseSwaggoInput true "Send input for requested signature"
// @Success 	200		{object}	model.BasicResponse
// @Router /api/signature-response [post]
func (s *Server) APISignatureResponse(req *ensweb.Request) *ensweb.Result {
	var resp did.SignRespData
	err := s.ParseJSON(req, &resp)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	dc := s.c.GetWebReq(resp.ID)
	if dc == nil {
		return s.BasicResponse(req, false, "Invalid request ID", nil)
	}
	s.c.UpateWebReq(resp.ID, req)
	dc.InChan <- resp
	return s.didResponse(req, resp.ID)
}

// APIGetPledgedTokenDetails godoc
// @Summary     Get details about the pledged tokens
// @Description This API allows the user to get details about the tokens the quorums have pledged i.e. which token is pledged for which token state
// @Tags        Account
// @Produce     json
// @Success     200 {object} model.TokenStateResponse
// @Router      /api/get-pledgedtoken-details [get]
func (s *Server) APIGetPledgedTokenDetails(req *ensweb.Request) *ensweb.Result {
	pledgedTokenInfo, err := s.c.GetPledgedInfo()
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	tokenstateresponse := model.TokenStateResponse{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got pledged tokens with token states info successfully",
		},
		PledgedTokenStateDetails: make([]model.PledgedTokenStateDetails, 0),
	}
	tokenstateresponse.PledgedTokenStateDetails = append(tokenstateresponse.PledgedTokenStateDetails, pledgedTokenInfo...)
	return s.RenderJSON(req, tokenstateresponse, http.StatusOK)
}

// APICheckPinnedState godoc
// @Summary     Check for exhausted token state hash
// @Description This API is used to check if the token state for which the token is pledged is exhausted or not.
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       tokenstatehash	query	string	true	"Token State Hash"
// @Success 	200		{object}	model.BasicResponse
// @Router /api/check-pinned-state [delete]
func (s *Server) APICheckPinnedState(req *ensweb.Request) *ensweb.Result {
	tokenstatehash := s.GetQuerry(req, "tokenstatehash")

	provList, err := s.c.GetDHTddrs(tokenstatehash)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	var br model.BasicResponse
	if len(provList) == 0 {
		br.Status = false
		br.Message = fmt.Sprintf("No pins available on %s", tokenstatehash)
		return s.RenderJSON(req, br, http.StatusOK)
	} else {
		br.Status = true
		br.Result = provList
	}

	err = s.c.UpdatePledgedTokenInfo(tokenstatehash)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	br.Message = "Got Pins on " + tokenstatehash + ". Updated the pledging detail in table and removed from pledged token state table."
	return s.RenderJSON(req, br, http.StatusOK)
}

func (s *Server) APIValidateTokenChain(req *ensweb.Request) *ensweb.Result {
	userDID := s.GetQuerry(req, "did")
	token := s.GetQuerry(req, "token")
	blockCountStr := s.GetQuerry(req, "blockcount")
	smartContractChainValidationStr := s.GetQuerry(req, "SCChainValidation")
	blockCount, err := strconv.Atoi(blockCountStr)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to convert blockCount string into integer", nil)
	}

	if userDID == "" {
		return s.BasicResponse(req, false, "user did is not provided", nil)
	}

	smartContractChainValidation, err := strconv.ParseBool(smartContractChainValidationStr)
	if err != nil {
		return s.BasicResponse(req, false, "Error converting string to boolean", nil)
	}

	var br *model.BasicResponse
	if smartContractChainValidation {
		s.log.Debug("validating smart contract")
		br, err = s.c.SmartContractTokenChainValidation(userDID, token, blockCount)
		if err != nil {
			return s.BasicResponse(req, false, br.Message, nil)
		}
	} else {
		s.log.Debug("validating rbt token")
		br, err = s.c.TokenChainValidation(userDID, token, blockCount)
		if err != nil {
			return s.BasicResponse(req, false, br.Message, nil)
		}
	}

	return s.RenderJSON(req, br, http.StatusOK)
}

func (s *Server) APIGenerateFaucetTestToken(req *ensweb.Request) *ensweb.Result {
	var tr model.FaucetRBTGenerateRequest
	err := s.ParseJSON(req, &tr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(tr.DID)
	if !strings.HasPrefix(tr.DID, "bafybmi") || len(tr.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	if tr.TokenCount <= 0 {
		s.log.Error("Invalid level number, level should be greater than 0")
		return s.BasicResponse(req, false, "Invalid level number, level should be greater than 0", nil)
	}

	if !s.validateDIDAccess(req, tr.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.GenerateFaucetTestTokens(req.ID, tr.TokenCount, tr.DID)
	return s.didResponse(req, req.ID)
}

func (s *Server) APIFaucetTokenCheck(req *ensweb.Request) *ensweb.Result {
	token := s.GetQuerry(req, "token")
	did := s.GetQuerry(req, "did")

	br := s.c.FaucetTokenCheck(token, did)
	return s.RenderJSON(req, br, http.StatusOK)
}

func (s *Server) APIValidateToken(req *ensweb.Request) *ensweb.Result {
	token := s.GetQuerry(req, "token")
	br, err := s.c.ValidateToken(token)
	if err != nil {
		s.log.Error("Failed to validate token ", err)
		return s.BasicResponse(req, false, "Failed to validate token : "+err.Error(), nil)
	}
	return s.RenderJSON(req, br, http.StatusOK)
}

// initiates transaction request from wallet server
func (s *Server) TxnReqFromWallet(txnReq *model.RBTTransferRequest, req *ensweb.Request) *ensweb.Result {
	is_alphanumeric_sender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(txnReq.Sender)
	is_alphanumeric_receiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(txnReq.Receiver)
	if !is_alphanumeric_sender || !is_alphanumeric_receiver {
		s.log.Error("Invalid sender or receiver address. Please provide valid DID")
		return s.BasicResponse(req, false, "Invalid sender or receiver address", nil)
	}
	if !strings.HasPrefix(txnReq.Sender, "bafybmi") || len(txnReq.Sender) != 59 || !strings.HasPrefix(txnReq.Receiver, "bafybmi") || len(txnReq.Receiver) != 59 {
		s.log.Error("Invalid sender or receiver DID")
		return s.BasicResponse(req, false, "Invalid sender or receiver DID", nil)
	}
	if txnReq.TokenCount < 0.001 {
		s.log.Error("Invalid RBT amount. RBT amount should be atlease 0.001")
		return s.BasicResponse(req, false, "Invalid RBT amount", nil)
	}
	if txnReq.Type < 1 || txnReq.Type > 2 {
		s.log.Error("Invalid trans type. TransType should be 1 or 2")
		return s.BasicResponse(req, false, "Invalid trans type", nil)
	}
	if !s.validateDIDAccess(req, txnReq.Sender) {
		return s.BasicResponse(req, false, "invalid sender DID access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateRBTTransfer(req.ID, txnReq)
	return s.didResponse(req, req.ID)
}

// initiates pre pledging of all free tokens
func (s *Server) APIPrePledge(req *ensweb.Request) *ensweb.Result {

	var prePledgeReq *model.CvrAPIRequest
	err := s.ParseJSON(req, &prePledgeReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input to pre-pledge request", nil)
	}

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(prePledgeReq.DID)
	if !strings.HasPrefix(prePledgeReq.DID, "bafybmi") || len(prePledgeReq.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}

	if !strings.HasPrefix(prePledgeReq.DID, "bafybmi") || len(prePledgeReq.DID) != 59 {
		s.log.Error("Invalid sender or receiver DID")
		return s.BasicResponse(req, false, "Invalid sender or receiver DID", nil)
	}

	if prePledgeReq.QuorumType < 1 || prePledgeReq.QuorumType > 2 {
		s.log.Error("Invalid trans type. TransType should be 1 or 2")
		return s.BasicResponse(req, false, "Invalid trans type. TransType should be 1 or 2", nil)
	}
	if !s.validateDIDAccess(req, prePledgeReq.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateRBTCVRTwo(req.ID, prePledgeReq)
	return s.didResponse(req, req.ID)
}

// initiates pre pledging of all free FTs
func (s *Server) APIPrePledgeFTs(req *ensweb.Request) *ensweb.Result {

	var prePledgeReq *model.CvrAPIRequest
	err := s.ParseJSON(req, &prePledgeReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input to pre-pledge request", nil)
	}

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(prePledgeReq.DID)
	if !strings.HasPrefix(prePledgeReq.DID, "bafybmi") || len(prePledgeReq.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}

	if !strings.HasPrefix(prePledgeReq.DID, "bafybmi") || len(prePledgeReq.DID) != 59 {
		s.log.Error("Invalid sender or receiver DID")
		return s.BasicResponse(req, false, "Invalid sender or receiver DID", nil)
	}

	if prePledgeReq.QuorumType < 1 || prePledgeReq.QuorumType > 2 {
		s.log.Error("Invalid trans type. TransType should be 1 or 2")
		return s.BasicResponse(req, false, "Invalid trans type. TransType should be 1 or 2", nil)
	}
	if !s.validateDIDAccess(req, prePledgeReq.DID) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateFTCVRTwo(req.ID, prePledgeReq)
	return s.didResponse(req, req.ID)
}