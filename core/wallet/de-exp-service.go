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

var ExplorerHost string

// Notification queue to prevent blocking
type NotificationQueue struct {
	queue   chan notificationTask
	done    chan struct{}
	wg      sync.WaitGroup
	workers int
}

type notificationTask struct {
	url     string
	payload []byte
	retries int
}

const (
	maxRetries      = 3
	retryBackoff    = 500 * time.Millisecond
	queueSize       = 1000
	numWorkers      = 4
	requestTimeout  = 15 * time.Second
	shutdownTimeout = 30 * time.Second
)

var notifQueue *NotificationQueue

func init() {
	notifQueue = NewNotificationQueue(numWorkers)
}

func NewNotificationQueue(workers int) *NotificationQueue {
	nq := &NotificationQueue{
		queue:   make(chan notificationTask, queueSize),
		done:    make(chan struct{}),
		workers: workers,
	}
	nq.startWorkers()
	return nq
}

func (nq *NotificationQueue) startWorkers() {
	for i := 0; i < nq.workers; i++ {
		nq.wg.Add(1)
		go nq.worker()
	}
}

func (nq *NotificationQueue) worker() {
	defer nq.wg.Done()
	client := initExplorerClient()

	for {
		select {
		case <-nq.done:
			return
		case task := <-nq.queue:
			nq.sendWithRetry(client, task)
		}
	}
}

func (nq *NotificationQueue) sendWithRetry(client *http.Client, task notificationTask) {
	for attempt := 0; attempt <= task.retries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)

		req, err := http.NewRequestWithContext(ctx, "POST", task.url, bytes.NewBuffer(task.payload))
		if err != nil {
			cancel()
			if attempt < task.retries {
				time.Sleep(retryBackoff * time.Duration(1+attempt))
				continue
			}
			return
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		cancel()

		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return // Success
			}
			// Log non-OK status but continue retrying
		}

		if attempt < task.retries {
			time.Sleep(retryBackoff * time.Duration(1+attempt))
		}
	}
}

func (nq *NotificationQueue) Enqueue(url string, payload []byte) error {
	// Recover from panic if queue is closed during shutdown
	defer func() {
		if r := recover(); r != nil {
			// Queue was closed, ignore
		}
	}()

	select {
	case nq.queue <- notificationTask{url: url, payload: payload, retries: maxRetries}:
		return nil
	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("notification queue full")
	default:
		return fmt.Errorf("notification queue closed")
	}
}

func (nq *NotificationQueue) Shutdown(ctx context.Context) error {
	close(nq.done)
	done := make(chan struct{})
	go func() {
		nq.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout")
	}
}

func initExplorerClient() *http.Client {
	clientOnce.Do(func() {
		explorerClient = &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   25,
				MaxConnsPerHost:       25,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableKeepAlives:     false,
				DisableCompression:    true,
			},
		}
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

func (w *Wallet) isExplorerAvailable() bool {
	return ExplorerHost != "" && ExplorerHost != "No De-Explorer Host"
}

func (w *Wallet) notifyExplorerServer(b *block.Block) {
	if !w.isExplorerAvailable() {
		return
	}

	explorerURL := ExplorerHost + setup.APINotifyDeExpBlockUpdate
	blockMap := b.GetBlockMap()
	cleanedMap := convertToStringMap(blockMap)

	blockBytes, err := json.Marshal(cleanedMap)
	if err != nil {
		w.log.Error("notifyExplorerServer: marshal failed", "error", err)
		return
	}

	// Queue async to avoid blocking
	if err := notifQueue.Enqueue(explorerURL, blockBytes); err != nil {
		w.log.Warn("Failed to queue block notification", "error", err)
	}
}

func (w *Wallet) notifyTokenUpdate(tableName string, tokenData interface{}, operation string) {
	if !w.isExplorerAvailable() {
		return
	}

	explorerURL := ExplorerHost + setup.APINotifyDeExpTokenUpdate

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

	// Queue async to avoid blocking
	if err := notifQueue.Enqueue(explorerURL, jsonBytes); err != nil {
		w.log.Warn("Failed to queue token notification", "error", err)
	}
}

// Add to your shutdown/cleanup code
func ShutdownExplorerNotifications() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return notifQueue.Shutdown(ctx)
}
