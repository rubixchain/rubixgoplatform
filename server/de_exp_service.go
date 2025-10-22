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
	did := s.GetQuerry(req, "did")
	if did == "" {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	tokens, err := s.c.GetRBTsbyDID(did)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get RBTs by DID", nil)
	}
	return s.BasicResponse(req, true, "RBTs fetched successfully", tokens)
}

func (s *Server) APIGetFTbyDID(req *ensweb.Request) *ensweb.Result {
	DID := s.GetQuerry(req, "did")
	if DID == "" {
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
	DID := s.GetQuerry(req, "did")
	if DID == "" {
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
	DID := s.GetQuerry(req, "did")
	if DID == "" {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	Tokens, err := s.c.GetSmartContractsbyDID(DID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get SmartContracts by DID", nil)
	}
	return s.BasicResponse(req, true, "SmartContracts fetched successfully", Tokens)
}

func (s *Server) APIGetFullTokenChain(req *ensweb.Request) *ensweb.Result {
	TokenID := s.GetQuerry(req, "tokenID")
	if TokenID == "" {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	TokenType := s.GetQuerry(req, "tokenType")
	if TokenType == "" {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(TokenID)
	if len(TokenID) != 46 || !strings.HasPrefix(TokenID, "Qm") || !is_alphanumeric {
		s.log.Error("Invalid RBT token")
		return s.BasicResponse(req, false, "Invalid FT token ID", nil)
	}
	getResp := s.c.GetTokenchain(TokenID, TokenType)
	return s.RenderJSON(req, getResp, http.StatusOK)
}

// func (s *Server) APIGetRBTFullTokenChain(req *ensweb.Request) *ensweb.Result {
// 	TokenID := s.GetQuerry(req, "tokenID")
// 	if TokenID == "" {
// 		return s.BasicResponse(req, false, "Invalid input", nil)
// 	}
// 	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(TokenID)
// 	if len(TokenID) != 46 || !strings.HasPrefix(TokenID, "Qm") || !is_alphanumeric {
// 		s.log.Error("Invalid RBT token")
// 		return s.BasicResponse(req, false, "Invalid FT token ID", nil)
// 	}
// 	getResp := s.c.GetRBTFullTokenchain(TokenID)
// 	return s.RenderJSON(req, getResp, http.StatusOK)
// }

// func (s *Server) APIGetFTFullTokenChain(req *ensweb.Request) *ensweb.Result {
// 	TokenID := s.GetQuerry(req, "tokenID")
// 	if TokenID == "" {
// 		return s.BasicResponse(req, false, "Invalid input", nil)
// 	}
// 	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(TokenID)
// 	if len(TokenID) != 46 || !strings.HasPrefix(TokenID, "Qm") || !is_alphanumeric {
// 		s.log.Error("Invalid FT token")
// 		return s.BasicResponse(req, false, "Invalid FT token ID", nil)
// 	}
// 	getResp := s.c.GetFTFullTokenchain(TokenID)
// 	return s.RenderJSON(req, getResp, http.StatusOK)
// }

// func (s *Server) APIGetRBTGenesisBlock(req *ensweb.Request) *ensweb.Result {
// 	TokenID := s.GetQuerry(req, "tokenID")
// 	if TokenID == "" {
// 		return s.BasicResponse(req, false, "Invalid input", nil)
// 	}
// 	genesisBlock := s.c.GetRBTTokenGenesisBlock(TokenID)
// 	// if err != nil {
// 	// 	return s.BasicResponse(req, false, "Failed to get tokens by DID", nil)
// 	// }
// 	return s.BasicResponse(req, true, "Free RBTs fetched successfully", genesisBlock)
// }

// func (s *Server) APIGetFTGenesisBlock(req *ensweb.Request) *ensweb.Result {
// 	TokenID := s.GetQuerry(req, "tokenID")
// 	if TokenID == "" {
// 		return s.BasicResponse(req, false, "Invalid input", nil)
// 	}
// 	genesisBlock := s.c.GetFTTokenGenesisBlock(TokenID)
// 	// if err != nil {
// 	// 	return s.BasicResponse(req, false, "Failed to get tokens by DID", nil)
// 	// }
// 	return s.BasicResponse(req, true, "Free RBTs fetched successfully", genesisBlock)
// }

// func (s *Server) APIGetRBTLatestBlock(req *ensweb.Request) *ensweb.Result {
// 	TokenID := s.GetQuerry(req, "tokenID")
// 	if TokenID == "" {
// 		return s.BasicResponse(req, false, "Invalid input", nil)
// 	}
// 	Tokens := s.c.GetRBTLatestBlock(TokenID)
// 	return s.BasicResponse(req, true, "Free RBTs fetched successfully", Tokens)
// }

// func (s *Server) APIGetFTLatestBlock(req *ensweb.Request) *ensweb.Result {
// 	TokenID := s.GetQuerry(req, "tokenID")
// 	if TokenID == "" {
// 		return s.BasicResponse(req, false, "Invalid input", nil)
// 	}
// 	Tokens := s.c.GetFTLatestBlock(TokenID)
// 	// if err != nil {
// 	// 	return s.BasicResponse(req, false, "Failed to get tokens by DID", nil)
// 	// }
// 	return s.BasicResponse(req, true, "Free RBTs fetched successfully", Tokens)
// }

// func (s *Server) APIGetRBTLatestValidators(req *ensweb.Request) *ensweb.Result {
// 	Tokens, err := s.c.GetTokensbyDID(DID)
// 	if err != nil {
// 		return s.BasicResponse(req, false, "Failed to get tokens by DID", nil)
// 	}
// 	return s.BasicResponse(req, true, "Free RBTs fetched successfully", Tokens)
// }

// func (s *Server) APIGetFTLatestValidators(req *ensweb.Request) *ensweb.Result {
// 	Tokens, err := s.c.GetTokensbyDID(DID)
// 	if err != nil {
// 		return s.BasicResponse(req, false, "Failed to get tokens by DID", nil)
// 	}
// 	return s.BasicResponse(req, true, "Free RBTs fetched successfully", Tokens)
// }
