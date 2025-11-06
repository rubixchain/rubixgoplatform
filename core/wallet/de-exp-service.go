package wallet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rubixchain/rubixgoplatform/block"
)

// convertToStringMap recursively converts map[interface{}]interface{} to map[string]interface{}
func convertToStringMap(i interface{}) interface{} {
	switch v := i.(type) {

	case map[interface{}]interface{}:
		m2 := make(map[string]interface{})
		for key, value := range v {
			m2[fmt.Sprintf("%v", key)] = convertToStringMap(value)
		}
		return m2

	case map[string]interface{}:
		m2 := make(map[string]interface{})
		for key, value := range v {
			m2[key] = convertToStringMap(value)
		}
		return m2

	case []interface{}:
		for idx, value := range v {
			v[idx] = convertToStringMap(value)
		}
		return v

	default:
		return v
	}
}

func (w *Wallet) notifyExplorerServer(b *block.Block) {
	explorerURL := "http://localhost:8080/api/block-update"

	blockMap := b.GetBlockMap()
	cleanedMap := convertToStringMap(blockMap)

	blockBytes, err := json.Marshal(cleanedMap)
	if err != nil {
		w.log.Error("Failed to marshal block map: %v\n", err)
		return
	}

	resp, err := http.Post(explorerURL, "application/json", bytes.NewBuffer(blockBytes))
	if err != nil {
		w.log.Error("Failed to send block to explorer: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.log.Error("Explorer server responded with status: %s\n", resp.Status)
	} else {
		fmt.Println("Block successfully sent to explorer.")
	}
}

// notifyTokenUpdate sends a token update notification to the Explorer server
func (w *Wallet) notifyTokenUpdate(tableName string, tokenData interface{}) {

	explorerURL := "http://localhost:8080/api/token-update"

	payload := map[string]interface{}{
		"table": tableName,
		"data":  tokenData,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		w.log.Error("Failed to marshal token update: %v", err)
		return
	}

	resp, err := http.Post(explorerURL, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		w.log.Error("Failed to send token update to explorer: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.log.Error("Explorer server responded with status: %s", resp.Status)
	} else {
		fmt.Println("✅ Token update successfully sent to explorer.")
	}
}
