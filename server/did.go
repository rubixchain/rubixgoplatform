package server

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const (
	DIDRootDir string = "root"
)

// CreateDID godoc
// @Summary      Create DID
// @Description  Creates a new DID with the provided public key, password, and mnemonic.
// @Tags         DID
// @Accept       json
// @Produce      json
// @Param        request  body      types.DIDCreate  true  "Create DID Request"
// @Success      200      {object}  models.BasicResponse
// @Failure      400      {object}  models.BasicResponse
// @Router       /rubix/v1/dids/create [post]
func (s *Server) APICreateDID(req *ensweb.Request) *ensweb.Result {
	var didCreate types.DIDCreate
	err := s.ParseJSON(req, &didCreate)
	if err != nil {
		s.log.Error("failed to parse did configuration", "err", err)
		return s.BasicResponse(req, false, "failed to parse did configuration", nil)
	}

	did, err := s.c.CreateDID(&didCreate)
	if err != nil {
		s.log.Error("failed to create did", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	didResp := models.DIDResult{
		DID:    did,
		PeerID: s.c.GetPeerID(),
	}
	return s.BasicResponse(req, true, "DID created successfully", didResp)
}

// GetAllDIDs godoc
// @Summary      Get All DIDs
// @Description  Retrieves a list of all DIDs.
// @Tags         DID
// @Accept       json
// @Produce      json
// @Success      200  {object}  models.BasicResponse
// @Failure      500  {object}  models.BasicResponse
// @Router       /rubix/v1/dids [get]
func (s *Server) APIGetAllDID(req *ensweb.Request) *ensweb.Result {
	ok := s.validateAccess(req)
	if !ok {
		return s.BasicResponse(req, false, "Unathuriozed access", nil)
	}

	didList := s.c.GetDIDs()
	didResponse := models.BasicResponse{
		Status:  true,
		Message: "Got all DIDs",
		Result:  didList,
	}
	return s.RenderJSON(req, &didResponse, http.StatusOK)
}

func (s *Server) validateDIDAccess(req *ensweb.Request, did string) bool {
	if s.cfg.EnableAuth {
		// always expect client token to present
		_ = req.ClientToken.Model.(*setup.BearerToken)
		return s.c.IsDIDExist(did)
	} else {
		return true
	}
}

func (s *Server) didResponse(req *ensweb.Request, reqID string) *ensweb.Result {
	dc := s.c.GetWebReq(reqID)
	ch := <-dc.OutChan
	time.Sleep(time.Millisecond * 10)
	sr, ok := ch.(*types.SignResponse)
	if ok {
		return s.RenderJSON(req, sr, http.StatusOK)
	}
	br, ok := ch.(*models.BasicResponse)
	if ok {
		s.c.RemoveWebReq(reqID)
		return s.RenderJSON(req, br, http.StatusOK)
	}
	return s.RenderJSON(req, &models.BasicResponse{Status: false, Message: "Invalid response"}, http.StatusOK)
}

// RegisterDID godoc
// @Summary      Register DID
// @Description  Registers a DID on the network.
// @Tags         DID
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID to register (e.g. did:bafybmih2cqn6okxy2sepgp75jq5dkopuohnbd3pfrrylmqnrz43ttihkky)"
// @Success      200  {object}  models.BasicResponse
// @Failure      400  {object}  models.BasicResponse
// @Router       /rubix/v1/dids/{did}/register [post]
func (s *Server) APIRegisterDID(req *ensweb.Request) *ensweb.Result {
	didStr := s.GetRouteVar(req, "did")
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(didStr)
	if !strings.HasPrefix(didStr, "bafybmi") || len(didStr) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID: %s", didStr)
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	s.c.AddWebReq(req)

	go s.c.RegisterDID(req.ID, didStr)
	return s.didResponse(req, req.ID)
}

// GetPubKeyByDID godoc
// @Summary      Get Public Key by DID
// @Description  Returns the hex-encoded public key a DID was derived from
// @Tags         DID
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID (e.g. bafybmih3l2emb4s7wbsgakwv4voaqngdirpg5f3kqlheqqsgdg7jthuwaq)"
// @Success      200  {object}  models.BasicResponse{result=models.DIDPublicKeyResult}
// @Failure      400  {object}  models.BasicResponse
// @Router       /rubix/v1/dids/{did}/public_key [get]
func (s *Server) APIGetPubKeyByDID(req *ensweb.Request) *ensweb.Result {
	didStr := s.GetRouteVar(req, "did")

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(didStr)
	if !strings.HasPrefix(didStr, "bafybmi") || len(didStr) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID:", didStr)
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}

	pubKeyResult, err := s.c.GetPubKeyByDID(didStr)
	if err != nil {
		s.log.Error("failed to get public key for did", "did", didStr, "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Got public key successfully", pubKeyResult)
}

func (s *Server) APISetupDID(req *ensweb.Request) *ensweb.Result {
	folderName, err := s.c.CreateTempFolder()
	if err != nil {
		s.log.Error("failed to create folder")
		return s.BasicResponse(req, false, "failed to create folder", nil)
	}
	defer os.RemoveAll(folderName)
	fileNames, fieldNames, err := s.ParseMultiPartForm(req, folderName+"/")
	if err != nil {
		s.log.Error("failed to parse request", "err", err)
		return s.BasicResponse(req, false, "failed to create DID", nil)
	}
	fields := fieldNames[setup.DIDConfigField]
	if len(fields) == 0 {
		s.log.Error("missing did configuration")
		return s.BasicResponse(req, false, "missing did configuration", nil)
	}
	var didCreate types.DIDCreate
	err = json.Unmarshal([]byte(fields[0]), &didCreate)
	if err != nil {
		s.log.Error("failed to parse did configuration", "err", err)
		return s.BasicResponse(req, false, "failed to parse did configuration", nil)
	}

	for _, fileName := range fileNames {

		if strings.Contains(fileName, constants.PvtKeyFileName) {
			didCreate.PrivKey = fileName
		}
		if strings.Contains(fileName, constants.PubKeyFileName) {
			didCreate.PubKey = fileName
		}
	}
	ok := s.validateAccess(req)
	if !ok {
		return s.BasicResponse(req, false, "Unathuriozed access", nil)
	}
	// didCreate.Dir = dir
	br := s.c.AddDID(&didCreate)
	return s.RenderJSON(req, br, http.StatusOK)
}

// arbitrary signature API
// @Summary     Request Arbitrary Signature
// @Description Accepts a DID and message to request an arbitrary signature asynchronously.
// @Tags        Signature
// @ID          arbitrary-signature
// @Accept      json
// @Produce     json
// @Param       input body models.ArbitrarySignRequest true "Arbitrary Signature Request"
// @Success     200 {object} models.BasicResponse
// @Failure     400 {object} models.BasicResponse
// @Router      /rubix/v1/signature/arbitrary [post]
func (s *Server) APIArbitrarySignature(req *ensweb.Request) *ensweb.Result {
	var signReq models.ArbitrarySignRequest
	err := s.ParseJSON(req, &signReq)
	if err != nil {
		s.log.Error("failed to parse sign input ", "err ", err)
		return s.BasicResponse(req, false, "arbitrary sign failed, failed to parse input", nil)
	}

	s.c.AddWebReq(req)
	go s.c.ArbitrarySign(req.ID, &signReq)
	return s.didResponse(req, req.ID)
}

// arbitrary signature verification API
// @Summary     Verify Arbitrary Signature
// @Description Verifies a signature for a given DID and signed message.
// @Tags        Signature
// @ID          verify-arbitrary-signature
// @Accept      json
// @Produce     json
// @Param       signer_did        query string true "DID of the signer"
// @Param       signed_msg   query string true "Signed message"
// @Param       signature  query string true "Signature to verify"
// @Success     200 {object} models.BasicResponse
// @Failure     400 {object} models.BasicResponse
// @Router      /rubix/v1/signature/verify [get]
func (s *Server) APISignVerification(req *ensweb.Request) *ensweb.Result {
	var verificationReq models.SignVerificationRequest
	verificationReq.SignerDID = s.GetQuery(req, "signer_did")
	verificationReq.SignedMsg = s.GetQuery(req, "signed_msg")
	verificationReq.Signature = s.GetQuery(req, "signature")

	verificationResp, err := s.c.ArbitrarySignVerification(req.ID, &verificationReq)
	if err != nil {
		s.log.Error("failed to verify given signature", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.RenderJSON(req, verificationResp, http.StatusOK)
}

// GetDIDBalance godoc
// @Summary      Get DID Balance
// @Description  Retrieves the overall balance (RBT, FT, NFT) for a given DID.
// @Tags         DID
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID (e.g. did:bafybmih3l2emb4s7wbsgakwv4voaqngdirpg5f3kqlheqqsgdg7jthuwaq)"
// @Success      200  {object}  models.BasicResponse
// @Failure      400  {object}  models.BasicResponse
// @Router       /rubix/v1/dids/{did}/balances [get]
func (s *Server) APIGetDIDBalance(req *ensweb.Request) *ensweb.Result {
	did := s.GetRouteVar(req, "did")
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID:", did)
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	assetsBalance := types.DIDBalances{
		DID: did,
	}
	ac := models.BasicResponse{
		Status: true,
	}
	rbtInfo, err := s.c.GetRbtByDid(did)
	if err != nil {
		ac.Message = "failed to get RBT balance, err :" + err.Error() + ";"
	} else {
		assetsBalance.RBTBalance = rbtInfo
	}
	ftInfo, err := s.c.GetFTInfoByDID(did)
	if err != nil {
		ac.Message += "failed to get FT balance, err :" + err.Error() + ";"
	} else {
		assetsBalance.FTBalance = ftInfo
	}
	nftInfo, err := s.c.GetNFTsByDid(did)
	if err != nil {
		ac.Message += "failed to get NFT balance, err :" + err.Error() + ";"
	} else {
		assetsBalance.NFTBalance = nftInfo
	}

	if ac.Message == "" {
		ac.Message = "Got account info successfully"
	}
	ac.Result = assetsBalance
	return s.RenderJSON(req, ac, http.StatusOK)
}
