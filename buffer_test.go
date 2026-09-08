package pinqloq

import "testing"

func newTestEntry(event, deviceIdentifier string) LogEntry {
	return LogEntry{Event: event, DeviceIdentifier: deviceIdentifier}
}

func TestLogBufferEnqueueStampsDate(t *testing.T) {
	buffer := newLogBuffer(10)
	entry := newTestEntry("test", "d1")

	if !buffer.enqueue(entry, nil, nil) {
		t.Fatal("expected enqueue to succeed")
	}
	if len(buffer.ch) != 1 {
		t.Fatalf("expected 1 item in channel, got %d", len(buffer.ch))
	}
}

func TestLogBufferDropsWhenFull(t *testing.T) {
	buffer := newLogBuffer(1)
	buffer.enqueue(newTestEntry("a", "d1"), nil, nil)

	var failedReason LogFailureReason
	onFailed := func(entry LogEntry, err *LogError) { failedReason = err.Reason }

	if buffer.enqueue(newTestEntry("b", "d1"), nil, onFailed) {
		t.Fatal("expected enqueue to fail when the buffer is full")
	}
	if failedReason != LogFailureReasonQueueFull {
		t.Fatalf("expected QueueFull, got %s", failedReason)
	}
}

func TestLogBufferEnqueueManyReportsWrittenCount(t *testing.T) {
	buffer := newLogBuffer(2)
	entries := []LogEntry{newTestEntry("a", "d1"), newTestEntry("b", "d1"), newTestEntry("c", "d1")}

	written := buffer.enqueueMany(entries, nil, nil)
	if written != 2 {
		t.Fatalf("expected 2 written, got %d", written)
	}
}

func TestLogBufferEnqueueAfterCloseFails(t *testing.T) {
	buffer := newLogBuffer(10)
	buffer.close()

	var failed bool
	onFailed := func(entry LogEntry, err *LogError) { failed = true }

	if buffer.enqueue(newTestEntry("a", "d1"), nil, onFailed) {
		t.Fatal("expected enqueue to fail after close")
	}
	if !failed {
		t.Fatal("expected onFailed to be called")
	}
}

func TestLogBufferDoubleCloseIsSafe(t *testing.T) {
	buffer := newLogBuffer(10)
	buffer.close()
	buffer.close()
}
