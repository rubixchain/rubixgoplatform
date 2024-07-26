package command

import (
	"strconv"
)

// Define a map for key translation
var keyMapping = map[string]string{
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
	"epoch": "TCEpoch",
	// Keys under "4"
	"4-1": "GBTypeKey",
	"4-2": "GBInfoKey",

	"4-2-1": "GITokenLevelKey",
	"4-2-2": "GITokenNumberKey",
	"4-2-3": "GIMigratedBlkIDKey",
	"4-2-4": "GIPreviousIDKey",
	"4-2-5": "GIParentIDKey",
	"4-2-6": "GIGrandParentIDKey",
	"4-2-7": "GICommitedTokensKey",
	"4-2-8": "GISmartContractValueKey",

	"5-1":    "TISenderDIDKey",
	"5-2":    "TIReceiverDIDKey",
	"5-3":    "TICommentKey",
	"5-4":    "TITIDKey",
	"5-5":    "TIBlockKey",
	"5-6":    "TITokensKey",
	"5-7":    "TIRefIDKey",
	"5-8":    "TIDeployerDIDKey",
	"5-9":    "TIExecutorDIDKey",
	"5-10":   "TICommitedTokensKey",
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

// flattenKeys processes the input recursively, flattening numeric keys and retaining non-numeric keys.
func flattenKeys(parentKey string, value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		flattenedMap := make(map[string]interface{})
		for k, nestedValue := range v {
			var newKey string
			if isInteger(k) {
				if parentKey != "" {
					newKey = parentKey + "-" + k
				} else {
					newKey = k
				}
				flattenedMap[newKey] = flattenKeys(newKey, nestedValue)
			} else {
				newKey = k
				flattenedMap[newKey] = flattenKeys(parentKey, nestedValue)
			}
		}
		return flattenedMap
	case []interface{}:
		for i, item := range v {
			v[i] = flattenKeys(parentKey, item)
		}
		return v
	default:
		return value
	}
}

// applyKeyMapping recursively applies the key mapping to the flattened JSON structure.
func applyKeyMapping(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		mappedMap := make(map[string]interface{})
		for k, nestedValue := range v {
			mappedKey, exists := keyMapping[k]
			if exists {
				mappedMap[mappedKey] = applyKeyMapping(nestedValue)
			} else {
				mappedMap[k] = applyKeyMapping(nestedValue)
			}
		}
		return mappedMap
	case []interface{}:
		for i, item := range v {
			v[i] = applyKeyMapping(item)
		}
		return v
	default:
		return value
	}
}

// isInteger checks if the given string is an integer.
func isInteger(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
