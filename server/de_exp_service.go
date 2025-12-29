package server

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIGetAllFreeRBT(req *ensweb.Request) *ensweb.Result {
	RBTs, err := s.c.GetAllRBTs()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get tokens", nil)
	}
	return s.BasicResponse(req, true, "Free RBTs fetched successfully", RBTs)
}

func (s *Server) APIGetAllFreeFTs(req *ensweb.Request) *ensweb.Result {
	FTs, err := s.c.GetAllFTs()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get tokens", nil)
	}
	return s.BasicResponse(req, true, "Free RBTs fetched successfully", FTs)
}

// to fetch the info for all the NFTs
func (s *Server) APIGetAllFreeNFTs(req *ensweb.Request) *ensweb.Result {
	NFTs, err := s.c.GetAllNFTs()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get tokens", nil)
	}
	return s.BasicResponse(req, true, "Free NFTs fetched successfully", NFTs)
}

// to fetch the info for all the NFTs
func (s *Server) APIGetAllFreeSmartContracts(req *ensweb.Request) *ensweb.Result {
	SmartContracts, err := s.c.GetAllSmartContracts()
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get Smart contracts", nil)
	}
	return s.BasicResponse(req, true, "Free Smart Contracts fetched successfully", SmartContracts)
}

func (s *Server) APIGetRBTbyDID(req *ensweb.Request) *ensweb.Result {
	did := strings.TrimSpace(s.GetQuerry(req, "did"))
	if strings.Compare(did, "") == 0 {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	tokens, err := s.c.GetRBTsbyDID(did)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get RBTs by DID", nil)
	}
	return s.BasicResponse(req, true, "RBTs fetched successfully", tokens)
}

func (s *Server) APIGetFTbyDID(req *ensweb.Request) *ensweb.Result {
	DID := strings.TrimSpace(s.GetQuerry(req, "did"))
	if strings.Compare(DID, "") == 0 {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	Tokens, err := s.c.GetFTsbyDID(DID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get FTs by DID", nil)
	}
	return s.BasicResponse(req, true, "FTs fetched successfully", Tokens)
}

// to fetch the NFTs(syncedNFT) by DID
func (s *Server) APIGetNFTbyDID(req *ensweb.Request) *ensweb.Result {
	DID := strings.TrimSpace(s.GetQuerry(req, "did"))
	if strings.Compare(DID, "") == 0 {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	Tokens, err := s.c.GetNFTsbyDID(DID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get NFTs by DID", nil)
	}
	return s.BasicResponse(req, true, "NFTs fetched successfully", Tokens)
}

// to fetch the SmartContracts(syncedSmartContract) by DID
func (s *Server) APIGetSmartContractbyDID(req *ensweb.Request) *ensweb.Result {
	DID := strings.TrimSpace(s.GetQuerry(req, "did"))
	if strings.Compare(DID, "") == 0 {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	Tokens, err := s.c.GetSmartContractsbyDID(DID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get SmartContracts by DID", nil)
	}
	return s.BasicResponse(req, true, "SmartContracts fetched successfully", Tokens)
}

func (s *Server) APIGetFullTokenChain(req *ensweb.Request) *ensweb.Result {
	TokenID := strings.TrimSpace(s.GetQuerry(req, "tokenID"))
	TokenType := strings.TrimSpace(s.GetQuerry(req, "tokenType"))

	// Log entry point
	s.log.Info("APIGetFullTokenChain called", "TokenID", TokenID, "TokenType", TokenType)

	if strings.Compare(TokenID, "") == 0 || strings.Compare(TokenType, "") == 0 {
		s.log.Error("Invalid input for APIGetFullTokenChain", "TokenID", TokenID, "TokenType", TokenType)
		return s.BasicResponse(req, false, "Invalid input", nil)
	}

	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(TokenID)
	if len(TokenID) != 46 || !strings.HasPrefix(TokenID, "Qm") || !isAlphanumeric {
		s.log.Error("Invalid TokenID format", "TokenID", TokenID, "TokenType", TokenType)
		return s.BasicResponse(req, false, "Invalid FT token ID", nil)
	}

	s.log.Info("Fetching token chain", "TokenID", TokenID, "TokenType", TokenType)
	getResp := s.c.GetTokenchain(TokenID, TokenType)

	if getResp == nil {
		s.log.Error("GetTokenchain returned nil response", "TokenID", TokenID, "TokenType", TokenType)
		return s.BasicResponse(req, false, "Failed to fetch token chain", nil)
	}

	s.log.Info("Tokenchain fetched successfully", "TokenID", TokenID, "TokenType", TokenType)
	return s.RenderJSON(req, getResp, http.StatusOK)
}

func (s *Server) APIGetTxnAmountFromFullNode(req *ensweb.Request) *ensweb.Result {
	txnID := strings.TrimSpace(s.GetQuerry(req, "txnID"))
	if strings.Compare(txnID, "") == 0 {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	Tokens, err := s.c.GettxnAmountFromFullNode(txnID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get txnAmount from full node", nil)
	}
	return s.BasicResponse(req, true, "Got transaction amount successfully", Tokens)
}

func (s *Server) APIGetFullTokenChainHeight(req *ensweb.Request) *ensweb.Result {
	tokenID := strings.TrimSpace(s.GetQuerry(req, "tokenID"))
	if strings.Compare(tokenID, "") == 0 {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}

	tokenType := strings.TrimSpace(s.GetQuerry(req, "tokenType"))
	if strings.Compare(tokenType, "") == 0 {
		return s.BasicResponse(req, false, "Invalid input token type", nil)
	}

	blockHeight, err := s.c.GetFullTokenChainHeight(tokenID, tokenType)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get token chain height", nil)
	}

	return s.BasicResponse(req, true, "Got token chain height", blockHeight)
}
