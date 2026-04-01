package server

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const (
	DIDRootDir string = "root"
)

func (s *Server) APIGetDIDAccess(req *ensweb.Request) *ensweb.Result {
	var da model.GetDIDAccess
	err := s.ParseJSON(req, &da)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid request", nil)
	}
	resp := s.c.GetDIDAccess(&da)
	return s.RenderJSON(req, resp, http.StatusOK)
}

func (s *Server) APIGetDIDChallenge(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuery(req, "did")
	resp := s.c.GetDIDChallenge(did)
	return s.RenderJSON(req, resp, http.StatusOK)
}

// CreateDID godoc
// @Summary      Create DID
// @Description  Creates a new DID with the provided public key, password, and mnemonic.
// @Tags         dids
// @Accept       json
// @Produce      json
// @Param        request  body      types.DIDCreate  true  "Create DID Request"
// @Success      200      {object}  model.BasicResponse
// @Failure      400      {object}  model.BasicResponse
// @Router       /rubix/v1/dids/create [post]
func (s *Server) APICreateDID(req *ensweb.Request) *ensweb.Result {
	var didCreate types.DIDCreate
	err := s.ParseJSON(req, &didCreate)
	if err != nil {
		s.log.Error("failed to parse did configuration", "err", err)
		return s.BasicResponse(req, false, "failed to parse did configuration", nil)
	}

	// depending on pub key passed or not, direct the function call
	var didLocal bool
	if didCreate.PubKey == "" {
		didLocal = true // create key pair and did locally
	} else {
		didLocal = false // create did from the input public key
	}

	did, err := s.c.CreateDID(&didCreate, didLocal)
	if err != nil {
		s.log.Error("failed to create did", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	didResp := model.DIDResult{
		DID:    did,
		PeerID: s.c.GetPeerID(),
	}
	return s.BasicResponse(req, true, "DID created successfully", didResp)
}

// GetAllDIDs godoc
// @Summary      Get All DIDs
// @Description  Retrieves a list of all DIDs.
// @Tags         dids
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Failure      500  {object}  model.BasicResponse
// @Router       /rubix/v1/dids [get]
func (s *Server) APIGetAllDID(req *ensweb.Request) *ensweb.Result {
	ok := s.validateAccess(req)
	if !ok {
		return s.BasicResponse(req, false, "Unathuriozed access", nil)
	}

	didList := s.c.GetDIDs()
	didResponse := model.BasicResponse{
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
	br, ok := ch.(*model.BasicResponse)
	if ok {
		s.c.RemoveWebReq(reqID)
		return s.RenderJSON(req, br, http.StatusOK)
	}
	return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: "Invalid response"}, http.StatusOK)
}

// RegisterDID godoc
// @Summary      Register DID
// @Description  Registers a DID on the network.
// @Tags         dids
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID to register (e.g. did:bafybmih2cqn6okxy2sepgp75jq5dkopuohnbd3pfrrylmqnrz43ttihkky)"
// @Success      200  {object}  model.BasicResponse
// @Failure      400  {object}  model.BasicResponse
// @Router       /rubix/v1/dids/{did}/register [get]
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
// @Param       input body model.ArbitrarySignRequest true "Arbitrary Signature Request"
// @Success     200 {object} model.BasicResponse
// @Failure     400 {object} model.BasicResponse
// @Router      /rubix/v1/signature/arbitrary [post]
func (s *Server) APIArbitrarySignature(req *ensweb.Request) *ensweb.Result {
	var signReq model.ArbitrarySignRequest
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
// @Success     200 {object} model.BasicResponse
// @Failure     400 {object} model.BasicResponse
// @Router      /rubix/v1/signature/verify [get]
func (s *Server) APISignVerification(req *ensweb.Request) *ensweb.Result {
	var verificationReq model.SignVerificationRequest
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

func (s *Server) APIRemoveStaleDID(req *ensweb.Request) *ensweb.Result {
	var m map[string]interface{}
	err := s.ParseJSON(req, &m)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	di, ok := m["did"]
	if !ok {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	didStr, ok := di.(string)
	if !ok {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(didStr)
	if !strings.HasPrefix(didStr, "bafybmi") || len(didStr) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	s.c.AddWebReq(req)

	go s.c.RemoveStaleDIDFromNetwork(req.ID, didStr)
	return s.didResponse(req, req.ID)
}

// GetDIDBalance godoc
// @Summary      Get DID Balance
// @Description  Retrieves the overall balance (RBT, FT, NFT) for a given DID.
// @Tags         dids
// @Accept       json
// @Produce      json
// @Param        did  path      string  true  "DID (e.g. did:bafybmih3l2emb4s7wbsgakwv4voaqngdirpg5f3kqlheqqsgdg7jthuwaq)"
// @Success      200  {object}  model.BasicResponse
// @Failure      400  {object}  model.BasicResponse
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
	ac := model.BasicResponse{
		Status:  true,
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
