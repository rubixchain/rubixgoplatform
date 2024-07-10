package block

// Key-to-constant mapping (Note: No duplicate keys)
var KeyMap = map[string]string{
	"1":  "TCTokenTypeKey",
	"2":  "TCTransTypeKey",
	"3":  "TCTokenOwnerKey",
	"4":  "TCGenesisBlockKey",
	"5":  "TCTransInfoKey",
	"6":  "TCSmartContractKey",
	"7":  "TCQuorumSignatureKey",
	"8":  "TCPledgeDetailsKey",
	"9":  "TCSmartContractDataKey",
	"10": "TCTokenValueKey",
	"11": "TCChildTokensKey",
	"12": "TCSenderSignatureKey",
	"98": "TCBlockHashKey",
	"99": "TCSignatureKey",
	// Genesis Block keys
	"4-1": "GBTypeKey",
	//"4-2": GBPrevIDKey,
	"4-3": "GBInfoKey",
	// GenesisInfo keys
	"4-3-1": "GITokenLevelKey",
	"4-3-2": "GITokenNumberKey",
	"4-3-3": "GIMigratedBlkIDKey",
	"4-3-4": "GIPreviousIDKey",
	"4-3-5": "GIParentIDKey",
	"4-3-6": "GIGrandParentIDKey",
	"4-3-7": "GICommitedTokensKey",
	"4-3-8": "GISmartContractValueKey",
	// TransInfo keys
	"5-1":  "TISenderDIDKey",
	"5-2":  "TIReceiverDIDKey",
	"5-3":  "TICommentKey",
	"5-4":  "TITIDKey",
	"5-5":  "TIBlockKey",
	"5-6":  "TITokensKey",
	"5-7":  "TIRefIDKey",
	"5-8":  "TIDeployerDIDKey",
	"5-9":  "TIExecutorDIDKey",
	"5-10": "TICommitedTokensKey",
	// TransToken keys
	"5-6-1":  "TTTokenTypeKey",
	"5-6-2":  "TTPledgedTokenKey",
	"5-6-3":  "TTPledgedDIDKey",
	"5-6-4":  "TTBlockNumberKey",
	"5-6-5":  "TTPreviousBlockIDKey",
	"5-6-6":  "TTUnpledgedIDKey",
	"5-6-7":  "TTCommitedDIDKey",
	"5-10-1": "TTTokenTypeKey",
	"5-10-4": "TTBlockNumberKey",
	"5-10-5": "TTPreviousBlockIDKey",
	"5-10-6": "TTUnpledgedIDKey",
	"5-10-7": "TTCommitedDIDKey",
}

// DecodeNestedStructure: Recursive function to decode nested structures
func DecodeNestedStructure(key string, content interface{}) interface{} {

	switch content := content.(type) { // Type switch for better handling
	case map[string]interface{}:
		decodedMap := make(map[string]interface{})
		for nestedKey, nestedContent := range content {
			// Look up the key in the map for the current nesting level
			decodedKey, exists := KeyMap[nestedKey]
			if !exists {
				// For deeper nested structures, construct combined keys
				// based on the parent key and current nested key
				decodedKey, exists = KeyMap[key+"-"+nestedKey]
				if !exists {
					decodedKey = nestedKey
				}
			}
			// Recursive call with the correct key for the nested structure
			decodedMap[decodedKey] = DecodeNestedStructure(nestedKey, nestedContent)
		}
		return decodedMap
	case []interface{}: // Handle slices/arrays
		decodedSlice := make([]interface{}, len(content))
		for i, v := range content {
			decodedSlice[i] = DecodeNestedStructure(key, v) // Recursive call
		}
		// Log the decoded slice
		return decodedSlice
	default:
		return content // No special decoding needed for other types
	}
}
