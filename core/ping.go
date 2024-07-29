package core

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// PingRequest is the model for ping request
type PingRequest struct {
	Message string `json:"message"`
}

// PingResponse is the model for ping response
type PingResponse struct {
	model.BasicResponse
}

type GetPeerInfoResponse struct {
	PeerInfo wallet.DIDPeerMap
	model.BasicResponse
}

// PingSetup will setup the ping route
func (c *Core) PingSetup() {
	c.l.AddRoute(APIPingPath, "GET", c.PingRecevied)
	c.l.AddRoute(APIGetPeerInfoPath, "GET", c.GetPeerInfoResponse)
	c.l.AddRoute(APILockInvalidToken, "POST", c.LockInvalidTokenResponse)
}

// CheckQuorumStatusSetup will setup the ping route
func (c *Core) CheckQuorumStatusSetup() {
	c.l.AddRoute(APICheckQuorumStatusPath, "GET", c.CheckQuorumStatusResponse)
}

// GetPeerdidTypeSetup will setup the ping route
func (c *Core) GetPeerdidTypeSetup() {
	c.l.AddRoute(APIGetPeerDIDTypePath, "GET", c.GetPeerdidTypeResponse)
}

// PingRecevied is the handler for ping request
func (c *Core) PingRecevied(req *ensweb.Request) *ensweb.Result {
	c.log.Info("Ping Received")
	resp := &PingResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	resp.Message = "Ping Received"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

// PingPeer will ping the peer & get the response
func (c *Core) PingPeer(peerID string) (string, error) {
	p, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		return "", err
	}
	// Close the p2p before exit
	defer p.Close()
	var pingResp PingResponse
	err = p.SendJSONRequest("GET", APIPingPath, nil, nil, &pingResp, false, 2*time.Minute)
	if err != nil {
		return "", err
	}
	return pingResp.Message, nil
}

// CheckQuorumStatusResponse is the handler for CheckQuorumStatus request
func (c *Core) CheckQuorumStatusResponse(req *ensweb.Request) *ensweb.Result { //PingRecevied
	did := c.l.GetQuerry(req, "did")
	c.log.Info("Checking Quorum Status")
	resp := &PingResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	_, ok := c.qc[did]
	if !ok {
		c.log.Error("Quorum is not setup")
		resp.Message = "Quorum is not setup"
		resp.Status = false
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	} else {
		resp.Status = true
		resp.Message = "Quorum is setup"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

}

// CheckQuorumStatus will ping the peer & get the response
func (c *Core) CheckQuorumStatus(peerID string, did string) (string, bool, error) { //
	q := make(map[string]string)
	if peerID == "" {
		peerID = c.qm.GetPeerID(did)
	}
	p, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		return "Quorum Connection Error", false, fmt.Errorf("quorum connection error")
	}
	// Close the p2p before exit
	defer p.Close()
	q["did"] = did
	var checkQuorumStatusResponse PingResponse
	err = p.SendJSONRequest("GET", APICheckQuorumStatusPath, q, nil, &checkQuorumStatusResponse, false, 2*time.Minute)
	if err != nil {
		return "Send Json Request error ", false, err
	}
	return checkQuorumStatusResponse.Message, checkQuorumStatusResponse.Status, nil
}

// CheckQuorumStatusResponse is the handler for CheckQuorumStatus request
func (c *Core) GetPeerdidTypeResponse(req *ensweb.Request) *ensweb.Result { //PingRecevied
	did := c.l.GetQuerry(req, "did")
	peer_peerid := c.l.GetQuerry(req, "self_peerid")
	peer_did := c.l.GetQuerry(req, "self_did")
	peer_did_type := c.l.GetQuerry(req, "self_did_type")

	resp := &model.GetDIDTypeResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}

	//If the peer's DID type string is not empty, register the peer, if not already registered
	if peer_did_type != "" {
		peer_did_type_int, err1 := strconv.Atoi(peer_did_type)
		if err1 != nil {
			c.log.Debug("could not convert string to integer:", err1)
		}

		err2 := c.w.AddDIDPeerMap(peer_did, peer_peerid, peer_did_type_int)
		if err2 != nil {
			c.log.Debug("could not add quorum details to DID peer table:", err2)
		}
	}

	dt, err := c.w.GetDID(did)
	if err != nil {
		c.log.Error("Couldn't fetch did type from DID Table", "error", err)
		resp.Message = "Couldn't fetch did type for did: " + did
		resp.Status = false
		resp.DidType = -1
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	} else {
		resp.DidType = dt.Type
		resp.Status = true
		resp.Message = "successfully fetched did type"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

}

// GetPeerdidType will ping the peer & get the did type
func (c *Core) GetPeerdidType_fromPeer(peerID string, peer_did string, self_DID string) (int, string, error) {
	q := make(map[string]string)
	p, err := c.pm.OpenPeerConn(peerID, peer_did, c.getCoreAppName(peerID))
	if err != nil {
		return -1, "Quorum Connection Error", fmt.Errorf("quorum connection error")
	}

	// Close the p2p before exit
	defer p.Close()
	q["did"] = peer_did

	if self_DID != "" {
		q["self_peerid"] = c.peerID
		q["self_did"] = self_DID
		self_dt, err := c.w.GetDID(self_DID)
		if err != nil {
			c.log.Info("could not fetch did type of peer:", self_DID)
		} else {
			q["self_did_type"] = strconv.Itoa(self_dt.Type)
		}
	}

	var getPeerdidTypeResponse model.GetDIDTypeResponse
	err = p.SendJSONRequest("GET", APIGetPeerDIDTypePath, q, nil, &getPeerdidTypeResponse, false, 2*time.Minute)
	if err != nil {
		return -1, "Send Json Request error ", err
	}
	return getPeerdidTypeResponse.DidType, getPeerdidTypeResponse.Message, nil
}

func (c *Core) GetPeerInfoResponse(req *ensweb.Request) *ensweb.Result { //PingRecevied
	//fetch peer details from DIDPeerTable
	peerDID := c.l.GetQuerry(req, "did")

	resp := &GetPeerInfoResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	var pInfo wallet.DIDPeerMap

	pInfo.PeerID = c.w.GetPeerID(peerDID)
	if pInfo.PeerID == "" {
		c.log.Error("sender does not have prev pledged quorum in DIDPeerTable", peerDID)
		resp.Message = "Couldn't fetch peer id for did: " + peerDID
		resp.Status = false
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

	qDidType, err := c.w.GetPeerDIDType(peerDID)
	if err != nil || qDidType == -1 {
		c.log.Error("could not fetch did type for quorum:", peerDID, "error", err)
		pInfo.DIDType = nil
		resp.PeerInfo = pInfo
		resp.Status = true
		resp.Message = "could not fetch did type, only sharing peerId"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	} else {
		pInfo.DIDType = &qDidType
	}

	resp.PeerInfo = pInfo
	resp.Status = true
	resp.Message = "successfully fetched peer details"
	return c.l.RenderJSON(req, &resp, http.StatusOK)

}

func (c *Core) GetPeerInfo(p *ipfsport.Peer, peerDID string) (GetPeerInfoResponse, error) {
	q := make(map[string]string)
	q["did"] = peerDID

	var response GetPeerInfoResponse
	err := p.SendJSONRequest("GET", APIGetPeerInfoPath, q, nil, &response, false)
	return response, err
}

// LockInvalidTokenResponse is the handler for LockInvalidToken request
func (c *Core) LockInvalidTokenResponse(req *ensweb.Request) *ensweb.Result { //PingRecevied
	resp := &model.BasicResponse{
		Status: false,
	}

	did := c.l.GetQuerry(req, "did")
	//exctract token Id and token type from the map - m
	var m map[string]string
	err := c.l.ParseJSON(req, &m)
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		resp.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}
	//exctract token Id
	tokenId, ok := m["token"]
	if !ok {
		c.log.Error("Missing old did value")
		resp.Message = "Missing old did value"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}
	//exctract token type
	token_type_str, ok := m["token_type"]
	if !ok {
		c.log.Error("Missing new did value")
		resp.Message = "Missing new did value"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}
	token_type, err := strconv.Atoi(token_type_str)
	if err != nil {
		resp.Message = "failed to retrieve token type"
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}
	if token_type == token.SmartContractTokenType {
		token_info, err := c.w.GetSmartContractToken(tokenId)
		if err != nil {
			resp.Message = "failed to retrieve token details. inValid token"
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		}
		sctoken := wallet.SmartContract{}
		for i := range token_info {
			if token_info[i].Deployer == did {
				sctoken = token_info[i]
				break
			}
		}
		if sctoken.ContractStatus != wallet.TokenIsDeployed {
			c.log.Error("Smart contract is not in deployed state, can not lock")
			resp.Message = "Smart contract is not in deployed state, can't lock"
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		}
		err = c.w.LockSmartContract(&sctoken)
		if err != nil {
			resp.Message = "failed to lock invalid smart contract"
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		}
		resp.Message = "Smart contract token locked successfully"

	} else { //token transferred or pledged/unpledged types
		token_info, err := c.w.ReadToken(tokenId)
		if err != nil {
			resp.Message = "failed to retrieve token details. inValid token"
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		}
		if token_info.DID != did || token_info.TokenStatus != wallet.TokenIsFree {
			c.log.Error("Not token owner, can not lock token")
			resp.Message = "Not token owner, can't lock token"
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		}
		err = c.w.LockToken(token_info)
		if err != nil {
			resp.Message = "failed to lock invalid token"
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		}
		resp.Message = "Token locked successfully"
	}

	resp.Status = true
	return c.l.RenderJSON(req, &resp, http.StatusOK)

}

func (c *Core) LockInvalidToken(tokenId string, tokenType int, user_did string) (*model.BasicResponse, error) {
	var resp model.BasicResponse
	p, err := c.pm.OpenPeerConn(c.peerID, user_did, c.getCoreAppName(c.peerID))
	if err != nil {
		resp.Message = "Self-peer Connection Error"
		resp.Status = false
		return &resp, err
	}
	m := make(map[string]string)
	m["token"] = tokenId
	m["token_type"] = strconv.Itoa(tokenType)
	err = p.SendJSONRequest("POST", APILockInvalidToken, nil, &m, &resp, true)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
