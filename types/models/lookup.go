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
var TokenStatusTypes []TokenStatus = []TokenStatus{
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

// CAUTION: DO NOT CHANGE THE ORDER SINCE THE 
// DATABASE ID IS GENERATED BASED ON THE POSITION IN THE ARRAY.
//
// NEW VALUES MUST BE APPENDED TO THE END OF THE ARRAY TO AVOID CHANGING EXISTING IDS.
var TokenTypeTypes []TokenType = []TokenType{
	{Name: constants.TokenType_RBT, IsActive: true},
	{Name: constants.TokenType_NFT, IsActive: true},
	{Name: constants.TokenType_FT, IsActive: true},
	{Name: constants.TokenType_SmartContract, IsActive: true},
}

