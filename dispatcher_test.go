package pinqloq

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeBatchSender struct {
	mu      sync.Mutex
	batches [][]queuedLog
	calls   atomic.Int64
	panic   bool
}

func (f *fakeBatchSender) sendBatch(ctx context.Context, items []queuedLog) {
	f.calls.Add(1)
	if f.panic {
		panic("boom")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	batch := make([]queuedLog, len(items))
	copy(batch, items)
	f.batches = append(f.batches, batch)
}

func (f *fakeBatchSender) batchLengths() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	lengths := make([]int, len(f.batches))
	for i, b := range f.batches {
		lengths[i] = len(b)
	}
	return lengths
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestLogDispatcherFlushesOnBatchSize(t *testing.T) {
	opts := Options{SecretKey: "sk", BatchSize: 2, FlushInterval: time.Minute, QueueCapacity: 10}
	buffer := newLogBuffer(opts.QueueCapacity)
	sender := &fakeBatchSender{}
	dispatcher := newLogDispatcher(buffer, sender, opts)
	dispatcher.start()

	buffer.enqueue(newTestEntry("a", "d1"), nil, nil)
	buffer.enqueue(newTestEntry("b", "d1"), nil, nil)

	waitUntil(t, 2*time.Second, func() bool { return sender.calls.Load() == 1 })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestLogDispatcherFlushesOnInterval(t *testing.T) {
	opts := Options{SecretKey: "sk", BatchSize: 10, FlushInterval: 50 * time.Millisecond, QueueCapacity: 10}
	buffer := newLogBuffer(opts.QueueCapacity)
	sender := &fakeBatchSender{}
	dispatcher := newLogDispatcher(buffer, sender, opts)
	dispatcher.start()

	buffer.enqueue(newTestEntry("a", "d1"), nil, nil)

	waitUntil(t, 2*time.Second, func() bool { return sender.calls.Load() == 1 })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dispatcher.shutdown(ctx)
}

func TestLogDispatcherShutdownDrainsRemaining(t *testing.T) {
	opts := Options{SecretKey: "sk", BatchSize: 10, FlushInterval: time.Minute, QueueCapacity: 10}
	buffer := newLogBuffer(opts.QueueCapacity)
	sender := &fakeBatchSender{}
	dispatcher := newLogDispatcher(buffer, sender, opts)
	dispatcher.start()

	buffer.enqueue(newTestEntry("a", "d1"), nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := dispatcher.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if sender.calls.Load() != 1 {
		t.Fatalf("expected 1 send call, got %d", sender.calls.Load())
	}
	lengths := sender.batchLengths()
	if len(lengths) != 1 || lengths[0] != 1 {
		t.Fatalf("expected one batch of 1, got %v", lengths)
	}
}

func TestLogDispatcherSurvivesSenderPanic(t *testing.T) {
	opts := Options{SecretKey: "sk", BatchSize: 1, FlushInterval: time.Minute, QueueCapacity: 10}
	buffer := newLogBuffer(opts.QueueCapacity)
	sender := &fakeBatchSender{panic: true}
	dispatcher := newLogDispatcher(buffer, sender, opts)
	dispatcher.start()

	buffer.enqueue(newTestEntry("a", "d1"), nil, nil)
	waitUntil(t, 2*time.Second, func() bool { return sender.calls.Load() == 1 })

	buffer.enqueue(newTestEntry("b", "d1"), nil, nil)
	waitUntil(t, 2*time.Second, func() bool { return sender.calls.Load() == 2 })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := dispatcher.shutdown(ctx); err != nil {
		t.Fatalf("dispatcher goroutine did not survive the panic: %v", err)
	}
}
