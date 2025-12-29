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
	url            string
	payload        []byte
	totalTimeout   time.Duration
	requestTimeout time.Duration
	retrySchedule  []time.Duration
}

const (
	queueSize             = 1000
	numWorkers            = 4
	defaultRequestTimeout = 15 * time.Second
	defaultTotalTimeout   = 3 * time.Minute
	shutdownTimeout       = 30 * time.Second
	enqueueTimeout        = 100 * time.Millisecond
)

var notifQueue *NotificationQueue

// Dynamic retry schedule: 5s, 30s, 1m
// Total: 5 + 30 + 60 = 95 seconds of waiting
// Plus 4 requests × 15s = 60 seconds = 155 seconds total (~2.5 minutes within 3-minute budget)
var defaultRetrySchedule = []time.Duration{
	5 * time.Second,  // 1st retry after 5 seconds
	30 * time.Second, // 2nd retry after 30 seconds
	60 * time.Second, // 3rd retry after 1 minute
}

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
			nq.sendWithDynamicRetry(client, task)
		}
	}
}

func (nq *NotificationQueue) sendWithDynamicRetry(client *http.Client, task notificationTask) {
	// Create overall context with total timeout
	overallCtx, cancel := context.WithTimeout(context.Background(), task.totalTimeout)
	defer cancel()

	numRetries := len(task.retrySchedule)
	maxAttempts := numRetries + 1 // 1 initial + N retries

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Check if overall timeout exceeded
		select {
		case <-overallCtx.Done():
			return // Total timeout exceeded
		default:
		}

		// Create request-specific context with request timeout
		reqCtx, reqCancel := context.WithTimeout(overallCtx, task.requestTimeout)

		req, err := http.NewRequestWithContext(reqCtx, "POST", task.url, bytes.NewBuffer(task.payload))
		if err != nil {
			reqCancel()
			if attempt < numRetries {
				nq.waitWithContext(overallCtx, task.retrySchedule[attempt], attempt+1)
				continue
			}
			return
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		reqCancel()

		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return // Success
			}
			// Log non-OK status but continue retrying
		}

		// If more retries available and overall timeout not exceeded
		if attempt < numRetries {
			nq.waitWithContext(overallCtx, task.retrySchedule[attempt], attempt+1)
		}
	}
}

// waitWithContext waits for the specified duration or until context is done
func (nq *NotificationQueue) waitWithContext(ctx context.Context, duration time.Duration, retryNum int) {
	select {
	case <-time.After(duration):
		// Wait completed, proceed to next retry
	case <-ctx.Done():
		// Overall timeout exceeded
	}
}

func (nq *NotificationQueue) Enqueue(url string, payload []byte) error {
	return nq.EnqueueWithConfig(url, payload, defaultTotalTimeout, defaultRequestTimeout, defaultRetrySchedule)
}

// EnqueueWithConfig allows custom timeout and retry schedule
func (nq *NotificationQueue) EnqueueWithConfig(url string, payload []byte, totalTimeout time.Duration, requestTimeout time.Duration, retrySchedule []time.Duration) error {
	// Validate timeout configuration
	if totalTimeout < requestTimeout {
		return fmt.Errorf("total timeout (%v) must be >= request timeout (%v)", totalTimeout, requestTimeout)
	}

	// Calculate total retry wait time
	totalRetryWait := time.Duration(0)
	for _, delay := range retrySchedule {
		totalRetryWait += delay
	}

	maxAttempts := len(retrySchedule) + 1
	maxRequestTime := time.Duration(maxAttempts) * requestTimeout
	totalRequired := maxRequestTime + totalRetryWait

	if totalRequired > totalTimeout {
		return fmt.Errorf("config impossible: requires %v but total timeout is only %v", totalRequired, totalTimeout)
	}

	// Recover from panic if queue is closed during shutdown
	defer func() {
		if r := recover(); r != nil {
			// Queue was closed, ignore
		}
	}()

	select {
	case nq.queue <- notificationTask{
		url:            url,
		payload:        payload,
		totalTimeout:   totalTimeout,
		requestTimeout: requestTimeout,
		retrySchedule:  retrySchedule,
	}:
		return nil
	case <-time.After(enqueueTimeout):
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
			Timeout: defaultRequestTimeout,
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
