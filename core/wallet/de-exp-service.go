package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/setup"
)

var (
	explorerClient *http.Client
	clientOnce     sync.Once
)

// Explorer configuration
const (
	explorerHost = "http://localhost:8080"
)

func initExplorerClient() *http.Client {
	clientOnce.Do(func() {
		explorerClient = &http.Client{
			Timeout: 12 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   25,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
		// Optional: log once
		// log.GetLogger().Info("Shared explorer HTTP client initialized")
	})
	return explorerClient
}

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
	explorerURL := explorerHost + setup.APINotifyDeExpBlockUpdate

	blockMap := b.GetBlockMap()
	cleanedMap := convertToStringMap(blockMap)

	blockBytes, err := json.Marshal(cleanedMap)
	if err != nil {
		w.log.Error("notifyExplorerServer: marshal failed", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", explorerURL, bytes.NewBuffer(blockBytes))
	if err != nil {
		w.log.Error("notifyExplorerServer: create request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := initExplorerClient()
	resp, err := client.Do(req)
	if err != nil {
		w.log.Error("notifyExplorerServer: request failed", "error", err)
		return
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		w.log.Error("Block update failed", "status", resp.StatusCode, "duration", duration)
	} else {
		w.log.Debug("Block sent to explorer", "duration", duration)
	}
}

// notifyTokenUpdate sends token update notifications to the Explorer server with operation type
func (w *Wallet) notifyTokenUpdate(tableName string, tokenData interface{}, operation string) {
	start := time.Now()
	explorerURL := explorerHost + setup.APINotifyDeExpTokenUpdate

	payload := map[string]interface{}{
		"table":     tableName,
		"data":      tokenData,
		"operation": operation,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		w.log.Error("notifyTokenUpdate: marshal failed", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", explorerURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		w.log.Error("notifyTokenUpdate: create request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := initExplorerClient()
	resp, err := client.Do(req)
	if err != nil {
		w.log.Error("notifyTokenUpdate: request failed", "error", err)
		return
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		w.log.Warn("Token update failed", "status", resp.StatusCode, "duration", duration)
	} else {
		w.log.Debug("Token update sent", "operation", operation, "duration", duration)
	}
}
