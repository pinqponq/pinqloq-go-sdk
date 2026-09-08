package pinqloq

import (
	"errors"
	"time"
)

const ingestBaseAddress = "https://pinqloq-external-api.pinqponq.io"

const (
	defaultBulkPath      = "api/client-logs/bulk"
	defaultBatchSize     = 200
	defaultFlushInterval = 2 * time.Second
	defaultQueueCapacity = 10_000
	defaultHTTPTimeout   = 10 * time.Second
)

// Options configures a Client.
type Options struct {
	SecretKey             string
	APILogsCollectionName string
	BulkPath              string
	BatchSize             int
	FlushInterval         time.Duration
	QueueCapacity         int
	HTTPTimeout           time.Duration
	AppVersionName        string
	DeviceIdentifier      string
}

func resolveOptions(opts Options) (Options, error) {
	if opts.SecretKey == "" {
		return Options{}, errors.New("pinqloq: SecretKey is required")
	}

	resolved := opts

	if resolved.BulkPath == "" {
		resolved.BulkPath = defaultBulkPath
	}
	if resolved.BatchSize <= 0 {
		resolved.BatchSize = defaultBatchSize
	}
	if resolved.FlushInterval <= 0 {
		resolved.FlushInterval = defaultFlushInterval
	}
	if resolved.QueueCapacity <= 0 {
		resolved.QueueCapacity = defaultQueueCapacity
	}
	if resolved.HTTPTimeout <= 0 {
		resolved.HTTPTimeout = defaultHTTPTimeout
	}

	return resolved, nil
}
