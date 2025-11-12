package wallet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
    "context"
	"time"
	"io"
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
	start := time.Now()
	explorerURL := "http://localhost:8080/api/block-update"

	blockMap := b.GetBlockMap()
	cleanedMap := convertToStringMap(blockMap)

	blockBytes, err := json.Marshal(cleanedMap)
	if err != nil {
		w.log.Error("Failed to marshal block map: %v\n", err)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", explorerURL, bytes.NewBuffer(blockBytes))
	if err != nil {
		w.log.Error("Failed to create request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a client with timeout and connection pooling
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		w.log.Error("Failed to send block to explorer: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Drain response body to allow connection reuse
	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		w.log.Error("Explorer server responded with status: %s (took %v)\n", resp.Status, duration)
	} else {
		fmt.Printf("✅ Block successfully sent to explorer (took %v)\n", duration)
	}
}

// notifyTokenUpdate sends token update notifications to the Explorer server with operation type
func (w *Wallet) notifyTokenUpdate(tableName string, tokenData interface{}, operation string) {
	start := time.Now()
	explorerURL := "http://localhost:8080/api/token-update"

	payload := map[string]interface{}{
		"table":     tableName,
		"data":      tokenData,
		"operation": operation,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		w.log.Error("Failed to marshal token update: %v", err)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", explorerURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		w.log.Error("Failed to create request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a client with timeout and connection pooling
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		w.log.Error("Failed to send token update to explorer: %v", err)
		return
	}
	defer resp.Body.Close()

	// Drain response body to allow connection reuse
	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		w.log.Error("Explorer server responded with status: %s (took %v)", resp.Status, duration)
	} else {
		fmt.Printf("✅ Token %s successfully sent to explorer (took %v)\n", operation, duration)
	}
}