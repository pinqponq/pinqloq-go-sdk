package pinqloq

import (
	"context"
	"testing"
	"time"
)

func newTestClient(t *testing.T, sender batchSender) *Client {
	t.Helper()

	resolved, err := resolveOptions(Options{SecretKey: "sk_test", DeviceIdentifier: "d1"})
	if err != nil {
		t.Fatalf("resolveOptions failed: %v", err)
	}

	buffer := newLogBuffer(resolved.QueueCapacity)
	dispatcher := newLogDispatcher(buffer, sender, resolved)
	logger := &defaultLogger{buffer: buffer, dispatcher: dispatcher, options: resolved}
	dispatcher.start()

	return &Client{logger: logger, dispatcher: dispatcher, options: resolved}
}

func TestClientEnqueueIsAShortcutForLoggerEnqueue(t *testing.T) {
	sender := &fakeBatchSender{}
	client := newTestClient(t, sender)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Shutdown(ctx)
	}()

	ok, err := client.Enqueue(LogEntry{Event: "order.created"}, nil, nil)
	if err != nil || !ok {
		t.Fatalf("expected Enqueue to succeed, got ok=%v err=%v", ok, err)
	}
}

func TestClientEnqueueManyIsAShortcutForLoggerEnqueueMany(t *testing.T) {
	sender := &fakeBatchSender{}
	client := newTestClient(t, sender)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Shutdown(ctx)
	}()

	entries := []LogEntry{{Event: "order.created"}, {Event: "order.shipped"}}
	accepted, err := client.EnqueueMany(entries, nil, nil)
	if err != nil || accepted != len(entries) {
		t.Fatalf("expected EnqueueMany to accept %d entries, got accepted=%d err=%v", len(entries), accepted, err)
	}
}
