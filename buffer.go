package pinqloq

import (
	"log"
	"sync"
	"time"
)

type queuedLog struct {
	entry    LogEntry
	onSent   OnSent
	onFailed OnFailed
}

type logBuffer struct {
	mu           sync.Mutex
	ch           chan queuedLog
	closed       bool
	droppedCount int64
}

func newLogBuffer(capacity int) *logBuffer {
	return &logBuffer{ch: make(chan queuedLog, capacity)}
}

func (b *logBuffer) enqueue(entry LogEntry, onSent OnSent, onFailed OnFailed) bool {
	if entry.Date.IsZero() {
		entry.Date = time.Now().UTC()
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		raiseFailed(onFailed, entry, &LogError{
			Reason:  LogFailureReasonQueueFull,
			Message: "Log queue is closed; the log was dropped.",
		})
		return false
	}

	select {
	case b.ch <- queuedLog{entry: entry, onSent: onSent, onFailed: onFailed}:
		b.mu.Unlock()
		return true
	default:
		b.droppedCount++
		dropped := b.droppedCount
		b.mu.Unlock()

		if dropped == 1 || dropped%1000 == 0 {
			log.Printf("pinqloq: log queue is full; %d logs dropped so far.", dropped)
		}

		raiseFailed(onFailed, entry, &LogError{
			Reason:  LogFailureReasonQueueFull,
			Message: "Log queue is full; the log was dropped. Increase QueueCapacity or lower FlushInterval.",
		})
		return false
	}
}

func (b *logBuffer) enqueueMany(entries []LogEntry, onSent OnSent, onFailed OnFailed) int {
	written := 0
	for _, entry := range entries {
		if b.enqueue(entry, onSent, onFailed) {
			written++
		}
	}
	return written
}

func (b *logBuffer) close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true
	close(b.ch)
}

func (b *logBuffer) channel() <-chan queuedLog {
	return b.ch
}
