package server

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/setup"
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
	did := s.GetQuerry(req, "did")
	resp := s.c.GetDIDChallenge(did)
	return s.RenderJSON(req, resp, http.StatusOK)
}

// APICreateDID will create new DID
func (s *Server) APICreateDID(req *ensweb.Request) *ensweb.Result {

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
	var didCreate did.DIDCreate
	err = json.Unmarshal([]byte(fields[0]), &didCreate)
	if err != nil {
		s.log.Error("failed to parse did configuration", "err", err)
		return s.BasicResponse(req, false, "failed to parse did configuration", nil)
	}

	if didCreate.Type < 0 || didCreate.Type > 4 {
		s.log.Error("DID Type should be between 0 and 4")
		return s.BasicResponse(req, false, "DID Type should be between 0 and 4", nil)
	}

	for _, fileName := range fileNames {

		if strings.Contains(fileName, did.PubKeyFileName) {
			didCreate.PubKeyFile = fileName
		}

		if didCreate.Type != did.LiteDIDMode {
			if strings.Contains(fileName, did.ImgFileName) {
				didCreate.ImgFile = fileName
			}
			if strings.Contains(fileName, did.DIDImgFileName) {
				didCreate.DIDImgFileName = fileName
			}
			if strings.Contains(fileName, did.PubShareFileName) {
				didCreate.PubImgFile = fileName
			}
		}
	}
	if !s.cfg.EnableAuth {
		didCreate.Dir = DIDRootDir
	}
	did, err := s.c.CreateDID(&didCreate)
	if err != nil {
		s.log.Error("failed to create did", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	didResp := model.DIDResponse{
		Status:  true,
		Message: "DID created successfully",
		Result: model.DIDResult{
			DID:    did,
			PeerID: s.c.GetPeerID(),
		},
	}
	return s.RenderJSON(req, &didResp, http.StatusOK)
}

// APIGetAllDID will get all DID
func (s *Server) APIGetAllDID(req *ensweb.Request) *ensweb.Result {
	dir, ok := s.validateAccess(req)
	if !ok {
		return s.BasicResponse(req, false, "Unathuriozed access", nil)
	}
	if s.cfg.EnableAuth {
		// always expect client token to present
		token, ok := req.ClientToken.Model.(*setup.BearerToken)
		if ok {
			dir = token.DID
		}
	}
	dt := s.c.GetDIDs(dir)
	ai := model.GetAccountInfo{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got all DIDs",
		},
		AccountInfo: make([]model.DIDAccountInfo, 0),
	}
	for _, d := range dt {
		a, err := s.c.GetAccountInfo(d.DID)
		if err == nil {
			a.DIDType = d.Type
			ai.AccountInfo = append(ai.AccountInfo, a)
		} else {
			a.DIDType = d.Type
			ai.AccountInfo = append(ai.AccountInfo, model.DIDAccountInfo{DID: d.DID})
		}
	}
	return s.RenderJSON(req, &ai, http.StatusOK)
}

func (s *Server) validateDIDAccess(req *ensweb.Request, did string) bool {
	if s.cfg.EnableAuth {
		// always expect client token to present
		token := req.ClientToken.Model.(*setup.BearerToken)
		return s.c.IsDIDExist(token.DID, did)
	} else {
		return true
	}
}

func (s *Server) didResponse(req *ensweb.Request, reqID string) *ensweb.Result {
	dc := s.c.GetWebReq(reqID)
	ch := <-dc.OutChan
	time.Sleep(time.Millisecond * 10)
	sr, ok := ch.(*did.SignResponse)
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

func (s *Server) APIRegisterDID(req *ensweb.Request) *ensweb.Result {
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
	var didCreate did.DIDCreate
	err = json.Unmarshal([]byte(fields[0]), &didCreate)
	if err != nil {
		s.log.Error("failed to parse did configuration", "err", err)
		return s.BasicResponse(req, false, "failed to parse did configuration", nil)
	}

	if didCreate.Type < 0 || didCreate.Type > 4 {
		s.log.Error("DID Type should be between 0 and 4")
		return s.BasicResponse(req, false, "DID Type should be between 0 and 4", nil)
	}

	for _, fileName := range fileNames {

		if strings.Contains(fileName, did.PvtKeyFileName) {
			didCreate.PrivKeyFile = fileName
		}
		if strings.Contains(fileName, did.PubKeyFileName) {
			didCreate.PubKeyFile = fileName
		}

		if didCreate.Type != did.LiteDIDMode {
			if strings.Contains(fileName, did.DIDImgFileName) {
				didCreate.DIDImgFileName = fileName
			}
			if strings.Contains(fileName, did.PubShareFileName) {
				didCreate.PubImgFile = fileName
			}
			if strings.Contains(fileName, did.PvtShareFileName) {
				didCreate.PrivImgFile = fileName
			}
			if strings.Contains(fileName, did.QuorumPvtKeyFileName) {
				didCreate.QuorumPrivKeyFile = fileName
			}
			if strings.Contains(fileName, did.QuorumPubKeyFileName) {
				didCreate.QuorumPubKeyFile = fileName
			}
		}
	}
	dir, ok := s.validateAccess(req)
	if !ok {
		return s.BasicResponse(req, false, "Unathuriozed access", nil)
	}
	didCreate.Dir = dir
	br := s.c.AddDID(&didCreate)
	return s.RenderJSON(req, br, http.StatusOK)
}

type DIDFromPubKeySwaggoRequest struct {
	PubKey string `json:"public_key"`
}

// APICreateDIDFromPubKey creates a DID from the provided public key

// @Summary     Returns DID for corresponding public key
// @Description This API will returns DID for corresponding public key
// @Tags        Account
// @ID 			request-did-for-pubkey
// @Accept      json
// @Produce     json
// @Param 		input body DIDFromPubKeySwaggoRequest true "Get DID from Public Key"
// @Success 200 {object} model.DIDFromPubKeyResponse
// @Router /api/request-did-for-pubkey [post]
func (s *Server) APICreateDIDFromPubKey(req *ensweb.Request) *ensweb.Result {
	var didReq model.DIDFromPubKeyRequest
	err := s.ParseJSON(req, &didReq)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input to create did from pub key", nil)
	}

	//provide required data to create a new lite mode did
	didCreate := did.DIDCreate{
		Type:       did.LiteDIDMode,
		PubKeyFile: "",
	}

	if !s.cfg.EnableAuth {
		didCreate.Dir = DIDRootDir
	}

	//pass the public key and other required data to create a did
	did, err := s.c.CreateDIDFromPubKey(&didCreate, didReq.PubKey)
	if err != nil {
		s.log.Error("failed to create did from given pub key", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}

	// respond with the requested did along with the corr. public key
	didResp := model.DIDFromPubKeyResponse{
		DID: did,
	}
	return s.RenderJSON(req, didResp, http.StatusOK)
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
// @Router      /api/sign [post]
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
// @Router      /api/verify-signature [get]
func (s *Server) APISignVerification(req *ensweb.Request) *ensweb.Result {
	var verificationReq model.SignVerificationRequest
	verificationReq.SignerDID = s.GetQuerry(req, "signer_did")
	verificationReq.SignedMsg = s.GetQuerry(req, "signed_msg")
	verificationReq.Signature = s.GetQuerry(req, "signature")

	verificationResp, err := s.c.ArbitrarySignVerification(req.ID, &verificationReq)
	if err != nil {
		s.log.Error("failed to verify given signature", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.RenderJSON(req, verificationResp, http.StatusOK)
}
