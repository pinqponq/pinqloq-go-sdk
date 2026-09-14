package pinqloq

// Logger is the main interface the Pinqloq SDK uses to send logs.
type Logger interface {
	Enqueue(entry LogEntry, onSent OnSent, onFailed OnFailed) (bool, error)
	EnqueueMany(entries []LogEntry, onSent OnSent, onFailed OnFailed) (int, error)
}

type defaultLogger struct {
	buffer     *logBuffer
	dispatcher *logDispatcher
}

func (l *defaultLogger) Enqueue(entry LogEntry, onSent OnSent, onFailed OnFailed) (bool, error) {
	return l.buffer.enqueue(entry, onSent, onFailed), nil
}

func (l *defaultLogger) EnqueueMany(entries []LogEntry, onSent OnSent, onFailed OnFailed) (int, error) {
	return l.buffer.enqueueMany(entries, onSent, onFailed), nil
}
