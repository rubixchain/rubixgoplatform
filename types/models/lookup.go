package models


const DidAlgo_SECP256K1 = "secp256k1"

// CAUTION: DO NOT CHANGE THE ORDER SINCE THE 
// DATABASE ID IS GENERATED BASED ON THE POSITION IN THE ARRAY.
//
// NEW VALUES MUST BE APPENDED TO THE END OF THE ARRAY TO AVOID CHANGING EXISTING IDS.
var DidAlgoTypes []DIDAlgo = []DIDAlgo{
	{Name: DidAlgo_SECP256K1, IsActive: true},
}

const TokenRole_Mint = "mint"
const TokenRole_Transfer = "transfer"
const TokenRole_Execute = "execute"
const TokenRole_Deploy = "deploy"
const TokenRole_Burn = "burn"
const TokenRole_Commit = "commit" // TODO: need a better name for it
const TokenRole_Uncommit = "uncommit"
const TokenRole_Pledge = "pledge"
const TokenRole_Unpledge = "unpledge"

// CAUTION: DO NOT CHANGE THE ORDER SINCE THE 
// DATABASE ID IS GENERATED BASED ON THE POSITION IN THE ARRAY.
//
// NEW VALUES MUST BE APPENDED TO THE END OF THE ARRAY TO AVOID CHANGING EXISTING IDS.
var TokenStatusTypes []TokenStatus = []TokenStatus{
	{Name: TokenRole_Mint, IsActive: true},
	{Name: TokenRole_Transfer, IsActive: true},
	{Name: TokenRole_Execute, IsActive: true},
	{Name: TokenRole_Deploy, IsActive: true},
	{Name: TokenRole_Burn, IsActive: true},
	{Name: TokenRole_Commit, IsActive: true},
	{Name: TokenRole_Uncommit, IsActive: true},
	{Name: TokenRole_Pledge, IsActive: true},
	{Name: TokenRole_Unpledge, IsActive: true},
}


const TokenType_RBT = "rbt"
const TokenType_NFT = "nft"
const TokenType_FT = "ft"
const TokenType_SmartContract = "smart_contract"

// CAUTION: DO NOT CHANGE THE ORDER SINCE THE 
// DATABASE ID IS GENERATED BASED ON THE POSITION IN THE ARRAY.
//
// NEW VALUES MUST BE APPENDED TO THE END OF THE ARRAY TO AVOID CHANGING EXISTING IDS.
var TokenTypeTypes []TokenType = []TokenType{
	{Name: TokenType_RBT, IsActive: true},
	{Name: TokenType_NFT, IsActive: true},
	{Name: TokenType_FT, IsActive: true},
	{Name: TokenType_SmartContract, IsActive: true},
}

