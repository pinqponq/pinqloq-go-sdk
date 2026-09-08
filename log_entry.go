package pinqloq

import "time"

// LogLevel matches the server-side ClientLogLevel one to one.
type LogLevel int

const (
	LogLevelDebug       LogLevel = 1
	LogLevelInformation LogLevel = 2
	LogLevelWarning     LogLevel = 3
	LogLevelError       LogLevel = 4
	LogLevelFatal       LogLevel = 5
)

// LogSourceType matches the server-side ClientLogSourceType one to one.
type LogSourceType int

const (
	LogSourceTypeDevice  LogSourceType = 1
	LogSourceTypeBackend LogSourceType = 2
)

func (t LogSourceType) wireName() string {
	if t == LogSourceTypeDevice {
		return "Device"
	}
	return "Backend"
}

// LogFailureReason explains why a log could not be sent.
type LogFailureReason string

const (
	LogFailureReasonUnauthorized LogFailureReason = "Unauthorized"
	LogFailureReasonForbidden    LogFailureReason = "Forbidden"
	LogFailureReasonQueueFull    LogFailureReason = "QueueFull"
	LogFailureReasonHTTPError    LogFailureReason = "HttpError"
	LogFailureReasonTimeout      LogFailureReason = "Timeout"
	LogFailureReasonNetwork      LogFailureReason = "Network"
)

// LogError carries the reason a log could not be sent; passed to an OnFailed callback.
type LogError struct {
	Reason     LogFailureReason
	StatusCode int
	Message    string
	Cause      error
}

func (e *LogError) Error() string {
	return e.Message
}

// LogEntry is a single log record. The producer creates it and places it on the queue.
type LogEntry struct {
	LogLevel         LogLevel
	Event            string
	Date             time.Time
	AppVersionName   string
	DeviceIdentifier string
	LogSourceType    LogSourceType
	CollectionName   string
	CorrelationID    string
	Path             string
	Metadata         map[string]string
	Detail           map[string]string
}

// OnSent is called when a log has been successfully delivered.
type OnSent func(entry LogEntry)

// OnFailed is called when a log could not be sent.
type OnFailed func(entry LogEntry, err *LogError)
