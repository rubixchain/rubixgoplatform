package core

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
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
		fmt.Println("peerID is empty in CheckQuorumStatus")
		peerID = c.qm.GetPeerID(did, c.peerID)
		if peerID == "" {
			qPeerDIDInfo, err := c.GetPeerDIDInfo(did)
			if err != nil {
				return "Quorum Connection Error 1", false, fmt.Errorf("1 unable to find Quorum DID info and peer for %v", did)
			} else {
				fmt.Println("peerID did type in CheckQuorumStatus ", qPeerDIDInfo.DIDType)
			}
			peerID = qPeerDIDInfo.PeerID
		}
	}
	if peerID == "" {
		return "Quorum Connection Error", false, fmt.Errorf("2 unable to find Quorum DID info and peer for %v", did)
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
	peerPeerID := c.l.GetQuerry(req, "self_peerid")
	peerDID := c.l.GetQuerry(req, "selfDID")
	peerDIDType := c.l.GetQuerry(req, "selfDID_type")

	resp := &model.GetDIDTypeResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}

	//If the peer's DID type string is not empty, register the peer, if not already registered
	if peerDIDType != "" {
		peerDIDTypeInt, err1 := strconv.Atoi(peerDIDType)
		if err1 != nil {
			c.log.Debug("could not convert string to integer:", err1)
		}

		err2 := c.w.AddDIDPeerMap(peerDID, peerPeerID, peerDIDTypeInt)
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
func (c *Core) GetPeerdidTypeFromPeer(peerID string, peerDID string, selfDID string) (int, string, error) {
	q := make(map[string]string)
	p, err := c.pm.OpenPeerConn(peerID, peerDID, c.getCoreAppName(peerID))
	if err != nil {
		return -1, "Quorum Connection Error", fmt.Errorf("quorum connection error")
	}

	// Close the p2p before exit
	defer p.Close()
	q["did"] = peerDID

	if selfDID != "" {
		q["self_peerid"] = c.peerID
		q["selfDID"] = selfDID
		selfDetails, err := c.w.GetDID(selfDID)
		if err != nil {
			c.log.Info("could not fetch did type of peer:", selfDID)
		} else {
			q["selfDID_type"] = strconv.Itoa(selfDetails.Type)
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
		_, err := c.w.GetDID(peerDID)
		if err != nil {
			c.log.Error("sender does not have prev pledged quorum in DIDPeerTable", peerDID)
			resp.Message = "Couldn't fetch peer id for did: " + peerDID
			resp.Status = false
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		} else {
			pInfo.PeerID = c.peerID
		}
	}

	qDidType, err := c.w.GetPeerDIDType(peerDID)
	if err != nil || qDidType == -1 {
		if strings.Contains(err.Error(), "no records found") {
			didInfo, err := c.w.GetDID(peerDID)
			if err != nil {
				c.log.Error("unable to find DID in DIDTable, could not fetch did type for quorum:", peerDID, "error", err)
				pInfo.DIDType = nil
				resp.PeerInfo = pInfo
				resp.Status = true
				resp.Message = "could not fetch did type, only sharing peerId"
				return c.l.RenderJSON(req, &resp, http.StatusOK)
			} else {
				pInfo.DIDType = &didInfo.Type
			}
		} else {
			c.log.Error("could not fetch did type for quorum:", peerDID, "error", err)
			pInfo.DIDType = nil
			resp.PeerInfo = pInfo
			resp.Status = true
			resp.Message = "could not fetch did type, only sharing peerId"
			return c.l.RenderJSON(req, &resp, http.StatusOK)
		}
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



