package pinqloq

import (
	"errors"
	"strings"
)

var errDeviceIdentifierRequired = errors.New(
	"pinqloq: DeviceIdentifier is required; set it on the entry, or configure the global Options.DeviceIdentifier fallback",
)

// Logger is the main interface the Pinqloq SDK uses to send logs.
type Logger interface {
	Enqueue(entry LogEntry, onSent OnSent, onFailed OnFailed) (bool, error)
	EnqueueMany(entries []LogEntry, onSent OnSent, onFailed OnFailed) (int, error)
}

type defaultLogger struct {
	buffer     *logBuffer
	dispatcher *logDispatcher
	options    Options
}

func (l *defaultLogger) Enqueue(entry LogEntry, onSent OnSent, onFailed OnFailed) (bool, error) {
	if err := l.ensureDeviceIdentifier(entry); err != nil {
		return false, err
	}
	return l.buffer.enqueue(entry, onSent, onFailed), nil
}

func (l *defaultLogger) EnqueueMany(entries []LogEntry, onSent OnSent, onFailed OnFailed) (int, error) {
	for _, entry := range entries {
		if err := l.ensureDeviceIdentifier(entry); err != nil {
			return 0, err
		}
	}
	return l.buffer.enqueueMany(entries, onSent, onFailed), nil
}

func (l *defaultLogger) ensureDeviceIdentifier(entry LogEntry) error {
	if strings.TrimSpace(entry.DeviceIdentifier) != "" || strings.TrimSpace(l.options.DeviceIdentifier) != "" {
		return nil
	}
	return errDeviceIdentifierRequired
}
