package pinqloq

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeLogger struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (f *fakeLogger) Enqueue(entry LogEntry, onSent OnSent, onFailed OnFailed) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
	return true, nil
}

func (f *fakeLogger) EnqueueMany(entries []LogEntry, onSent OnSent, onFailed OnFailed) (int, error) {
	for _, e := range entries {
		f.Enqueue(e, onSent, onFailed)
	}
	return len(entries), nil
}

func buildTestClient(options Options) (*Client, *fakeLogger) {
	logger := &fakeLogger{}
	return &Client{logger: logger, options: options}, logger
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	io := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{}
	_ = json.NewDecoder(r.Body).Decode(&io)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": "abc123", "userId": "64"})
}

func TestRequestLoggingRejectsWithoutDeviceIdentifier(t *testing.T) {
	client, logger := buildTestClient(Options{SecretKey: "sk_test"})
	handler := client.RequestLogging(RequestLoggingOptions{})(http.HandlerFunc(loginHandler))

	req := httptest.NewRequest(http.MethodPost, "/users/login", strings.NewReader(`{"password":"hunter2"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(logger.entries) != 0 {
		t.Fatalf("expected no captured entries, got %d", len(logger.entries))
	}
}

func TestRequestLoggingCapturesMetadataAndRedactsBuiltInFloor(t *testing.T) {
	client, logger := buildTestClient(Options{SecretKey: "sk_test"})
	handler := client.RequestLogging(RequestLoggingOptions{})(http.HandlerFunc(loginHandler))

	req := httptest.NewRequest(http.MethodPost, "/users/login", strings.NewReader(`{"email":"a@b.com","password":"hunter2"}`))
	req.Header.Set("Device-Identifier", "device-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(logger.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(logger.entries))
	}

	entry := logger.entries[0]
	if entry.DeviceIdentifier != "device-1" {
		t.Fatalf("unexpected device identifier: %s", entry.DeviceIdentifier)
	}
	if entry.Path != "/users/login" {
		t.Fatalf("expected Path field set, got %q", entry.Path)
	}
	if entry.Event != "POST /users/login" {
		t.Fatalf("unexpected event: %s", entry.Event)
	}
	if _, ok := entry.Metadata["path"]; ok {
		t.Fatal("expected metadata.path to be absent, path is its own field")
	}
	if entry.Metadata["method"] != "POST" || entry.Metadata["statusCode"] != "200" {
		t.Fatalf("unexpected metadata: %v", entry.Metadata)
	}
	if _, ok := entry.Metadata["RequestPath"]; ok {
		t.Fatal("expected metadata.RequestPath to be absent")
	}

	var input map[string]string
	json.Unmarshal([]byte(entry.Detail["InputJson"]), &input)
	if input["password"] != redactedValue || input["email"] != "a@b.com" {
		t.Fatalf("unexpected InputJson redaction: %v", input)
	}

	var output map[string]string
	json.Unmarshal([]byte(entry.Detail["OutputJson"]), &output)
	if output["token"] != redactedValue || output["userId"] != "64" {
		t.Fatalf("unexpected OutputJson redaction: %v", output)
	}
}

func TestRequestLoggingSkipsExcludedPaths(t *testing.T) {
	client, logger := buildTestClient(Options{SecretKey: "sk_test", DeviceIdentifier: "fallback"})
	handler := client.RequestLogging(RequestLoggingOptions{ExcludePaths: []string{"/health"}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(logger.entries) != 0 {
		t.Fatalf("expected no entries for excluded path, got %d", len(logger.entries))
	}
}

func TestRequestLoggingUsesGlobalDeviceIdentifierFallback(t *testing.T) {
	client, logger := buildTestClient(Options{SecretKey: "sk_test", DeviceIdentifier: "fallback-device"})
	handler := client.RequestLogging(RequestLoggingOptions{})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }),
	)

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(logger.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(logger.entries))
	}
	if logger.entries[0].DeviceIdentifier != "fallback-device" {
		t.Fatalf("expected fallback device identifier, got %s", logger.entries[0].DeviceIdentifier)
	}
	if logger.entries[0].LogLevel != LogLevelError {
		t.Fatalf("expected Error log level for 500, got %d", logger.entries[0].LogLevel)
	}
}

func TestRequestLoggingRedactPathsMasksWholeBody(t *testing.T) {
	client, logger := buildTestClient(Options{SecretKey: "sk_test", DeviceIdentifier: "d1"})
	handler := client.RequestLogging(RequestLoggingOptions{RedactPaths: []string{"/users"}})(http.HandlerFunc(loginHandler))

	req := httptest.NewRequest(http.MethodPost, "/users/login", strings.NewReader(`{"email":"a@b.com","password":"hunter2"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var input map[string]string
	json.Unmarshal([]byte(logger.entries[0].Detail["InputJson"]), &input)
	if input["email"] != redactedValue || input["password"] != redactedValue {
		t.Fatalf("expected whole body masked, got %v", input)
	}
}

func TestRequestLoggingMetadataEventEnricherOverridesEntryEvent(t *testing.T) {
	client, logger := buildTestClient(Options{SecretKey: "sk_test", DeviceIdentifier: "d1"})
	handler := client.RequestLogging(RequestLoggingOptions{
		Metadata: map[string]EnricherFunc{
			"event": func(r *http.Request, statusCode int, headers http.Header) string { return "custom.event.name" },
		},
	})(http.HandlerFunc(loginHandler))

	req := httptest.NewRequest(http.MethodPost, "/users/login", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if logger.entries[0].Event != "custom.event.name" {
		t.Fatalf("expected overridden event, got %s", logger.entries[0].Event)
	}
	if _, ok := logger.entries[0].Metadata["event"]; ok {
		t.Fatal("expected metadata.event to be absent")
	}
}

func TestRequestLoggingDownstreamStillReceivesFullBody(t *testing.T) {
	client, _ := buildTestClient(Options{SecretKey: "sk_test", DeviceIdentifier: "d1"})

	var receivedBody string
	handler := client.RequestLogging(RequestLoggingOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0)
		buf := make([]byte, 1024)
		for {
			n, err := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if err != nil {
				break
			}
		}
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"hello":"world"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedBody != `{"hello":"world"}` {
		t.Fatalf("expected downstream to see the full body, got %q", receivedBody)
	}
}
