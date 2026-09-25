package models

import (
	"github.com/rubixchain/rubixgoplatform/constants"
)

// CAUTION: DO NOT CHANGE THE ORDER SINCE THE
// DATABASE ID IS GENERATED BASED ON THE POSITION IN THE ARRAY.
//
// NEW VALUES MUST BE APPENDED TO THE END OF THE ARRAY TO AVOID CHANGING EXISTING IDS.
var DidAlgoTypes []DIDAlgo = []DIDAlgo{
	{Name: constants.DidAlgo_SECP256K1, IsActive: true},
}

// CAUTION: DO NOT CHANGE THE ORDER SINCE THE
// DATABASE ID IS GENERATED BASED ON THE POSITION IN THE ARRAY.
//
// NEW VALUES MUST BE APPENDED TO THE END OF THE ARRAY TO AVOID CHANGING EXISTING IDS.
var TokenRoleTypes []TokenRole = []TokenRole{
	{Name: constants.TokenRole_Mint, IsActive: true},
	{Name: constants.TokenRole_Transfer, IsActive: true},
	{Name: constants.TokenRole_Execute, IsActive: true},
	{Name: constants.TokenRole_Deploy, IsActive: true},
	{Name: constants.TokenRole_Burn, IsActive: true},
	{Name: constants.TokenRole_Commit, IsActive: true},
	{Name: constants.TokenRole_Uncommit, IsActive: true},
	{Name: constants.TokenRole_Pledge, IsActive: true},
	{Name: constants.TokenRole_Unpledge, IsActive: true},
}

func GetTokenRoleID(tokenRole string) int {
	for idx, entry := range TokenRoleTypes {
		if entry.Name == tokenRole {
			return idx + 1
		}
	}

	return -1
}

// CAUTION: DO NOT CHANGE THE ORDER SINCE THE
// DATABASE ID IS GENERATED BASED ON THE POSITION IN THE ARRAY.
//
// NEW VALUES MUST BE APPENDED TO THE END OF THE ARRAY TO AVOID CHANGING EXISTING IDS.
var TokenTypeTypes []TokenType = []TokenType{
	{Name: constants.TokenType_RBT, IsActive: true},
	{Name: constants.TokenType_NFT, IsActive: true},
	{Name: constants.TokenType_FT, IsActive: true},
	{Name: constants.TokenType_SmartContract, IsActive: true},
	// Appended at the END so existing IDs 1-4 are unchanged; this is ID 5.
	{Name: constants.TokenType_Properties, IsActive: true},
}

func GetTokenTypeID(tokenType string) int {
	for idx, entry := range TokenTypeTypes {
		if entry.Name == tokenType {
			return idx + 1
		}
	}

	return -1
}
