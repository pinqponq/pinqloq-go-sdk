package pinqloq

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newTestQueuedLog(event, collectionName string, onSent OnSent, onFailed OnFailed) queuedLog {
	return queuedLog{
		entry: LogEntry{
			Event:            event,
			DeviceIdentifier: "d1",
			LogSourceType:    LogSourceTypeBackend,
			CollectionName:   collectionName,
			Date:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		onSent:   onSent,
		onFailed: onFailed,
	}
}

func withTestServer(t *testing.T, handler http.HandlerFunc) (*ingestAPIClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := newIngestAPIClient(Options{SecretKey: "sk_test", APILogsCollectionName: "app_logs", BulkPath: "api/client-logs/bulk", HTTPTimeout: 2 * time.Second})
	client.url = server.URL + "/api/client-logs/bulk"
	return client, server
}

func TestIngestClientPostsWirePayload(t *testing.T) {
	var capturedBody map[string]any
	var capturedSecret string
	var mu sync.Mutex

	client, _ := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		capturedSecret = r.Header.Get("X-Secret-Key")
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
	})

	var sentEvent string
	item := newTestQueuedLog("order.created", "", func(entry LogEntry) { sentEvent = entry.Event }, nil)

	client.sendBatch(context.Background(), []queuedLog{item})

	mu.Lock()
	defer mu.Unlock()
	if capturedSecret != "sk_test" {
		t.Fatalf("expected secret key header, got %q", capturedSecret)
	}
	if capturedBody["collectionName"] != "app_logs" {
		t.Fatalf("expected collectionName app_logs, got %v", capturedBody["collectionName"])
	}
	logs, ok := capturedBody["logs"].([]any)
	if !ok || len(logs) != 1 {
		t.Fatalf("expected 1 log, got %v", capturedBody["logs"])
	}
	logItem := logs[0].(map[string]any)
	if logItem["event"] != "order.created" || logItem["deviceIdentifier"] != "d1" || logItem["logSourceType"] != "Backend" {
		t.Fatalf("unexpected wire item: %v", logItem)
	}
	if sentEvent != "order.created" {
		t.Fatalf("expected onSent callback, got %q", sentEvent)
	}
}

func TestIngestClientGroupsByCollectionName(t *testing.T) {
	var mu sync.Mutex
	var requestCount int

	client, _ := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	items := []queuedLog{
		newTestQueuedLog("a", "col-a", nil, nil),
		newTestQueuedLog("b", "col-b", nil, nil),
	}
	client.sendBatch(context.Background(), items)

	mu.Lock()
	defer mu.Unlock()
	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}
}

func TestIngestClientUnauthorized(t *testing.T) {
	client, _ := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("nope"))
	})

	var failErr *LogError
	item := newTestQueuedLog("a", "", nil, func(entry LogEntry, err *LogError) { failErr = err })

	client.sendBatch(context.Background(), []queuedLog{item})

	if failErr == nil || failErr.Reason != LogFailureReasonUnauthorized || failErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected Unauthorized error, got %+v", failErr)
	}
}

func TestIngestClientNetworkError(t *testing.T) {
	client := newIngestAPIClient(Options{SecretKey: "sk", HTTPTimeout: 200 * time.Millisecond})
	client.url = "http://127.0.0.1:1/unreachable"

	var failErr *LogError
	item := newTestQueuedLog("a", "", nil, func(entry LogEntry, err *LogError) { failErr = err })

	client.sendBatch(context.Background(), []queuedLog{item})

	if failErr == nil || (failErr.Reason != LogFailureReasonNetwork && failErr.Reason != LogFailureReasonTimeout) {
		t.Fatalf("expected Network or Timeout error, got %+v", failErr)
	}
}
