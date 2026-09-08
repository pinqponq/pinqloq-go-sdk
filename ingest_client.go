package pinqloq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const secretKeyHeader = "X-Secret-Key"

const httpWarningThrottle = 60 * time.Second

const maxErrorBodyCharacters = 512

type wireLogItem struct {
	LogLevel         int               `json:"logLevel"`
	Event            string            `json:"event"`
	Date             string            `json:"date,omitempty"`
	AppVersionName   string            `json:"appVersionName,omitempty"`
	DeviceIdentifier string            `json:"deviceIdentifier"`
	LogSourceType    string            `json:"logSourceType"`
	CorrelationID    string            `json:"correlationId,omitempty"`
	Path             string            `json:"path,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	Detail           map[string]string `json:"detail,omitempty"`
}

type bulkWireRequest struct {
	CollectionName string        `json:"collectionName,omitempty"`
	Logs           []wireLogItem `json:"logs"`
}

type ingestAPIClient struct {
	options         Options
	url             string
	httpClient      *http.Client
	warnMu          sync.Mutex
	nextHTTPWarning time.Time
}

func newIngestAPIClient(options Options) *ingestAPIClient {
	return &ingestAPIClient{
		options:    options,
		url:        fmt.Sprintf("%s/%s", ingestBaseAddress, strings.TrimLeft(options.BulkPath, "/")),
		httpClient: &http.Client{Timeout: options.HTTPTimeout},
	}
}

func (c *ingestAPIClient) sendBatch(ctx context.Context, items []queuedLog) {
	if len(items) == 0 {
		return
	}

	groups := make(map[string][]queuedLog)
	var order []string
	for _, item := range items {
		collectionName := c.resolveCollectionName(item.entry)
		if _, ok := groups[collectionName]; !ok {
			order = append(order, collectionName)
		}
		groups[collectionName] = append(groups[collectionName], item)
	}

	for _, collectionName := range order {
		c.sendGroup(ctx, collectionName, groups[collectionName])
	}
}

func (c *ingestAPIClient) sendGroup(ctx context.Context, collectionName string, groupItems []queuedLog) {
	payload := bulkWireRequest{
		CollectionName: collectionName,
		Logs:           make([]wireLogItem, 0, len(groupItems)),
	}
	for _, item := range groupItems {
		payload.Logs = append(payload.Logs, c.toWireItem(item.entry))
	}

	body, err := json.Marshal(payload)
	if err != nil {
		c.failGroup(groupItems, c.buildExceptionError(err, false))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		c.failGroup(groupItems, c.buildExceptionError(err, false))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(secretKeyHeader, c.options.SecretKey)

	response, err := c.httpClient.Do(req)
	if err != nil {
		timeout := errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err)
		log.Printf("pinqloq: group of %d logs could not be sent (%s). %v", len(groupItems), c.formatCollectionName(collectionName), err)
		c.failGroup(groupItems, c.buildExceptionError(err, timeout))
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		for _, item := range groupItems {
			raiseSent(item.onSent, item.entry)
		}
		return
	}

	logError := c.buildHTTPError(response, collectionName)
	c.failGroup(groupItems, logError)
}

func isTimeoutError(err error) bool {
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

func (c *ingestAPIClient) failGroup(groupItems []queuedLog, logError *LogError) {
	for _, item := range groupItems {
		raiseFailed(item.onFailed, item.entry, logError)
	}
}

func (c *ingestAPIClient) buildHTTPError(response *http.Response, collectionName string) *LogError {
	status := response.StatusCode
	var reason LogFailureReason
	switch status {
	case http.StatusUnauthorized:
		reason = LogFailureReasonUnauthorized
	case http.StatusForbidden:
		reason = LogFailureReasonForbidden
	default:
		reason = LogFailureReasonHTTPError
	}

	var message string
	switch {
	case reason == LogFailureReasonUnauthorized:
		message = "Unauthorized (HTTP 401): the secret key is invalid or missing."
	case reason == LogFailureReasonForbidden:
		message = fmt.Sprintf("Forbidden (HTTP 403): the secret key is not authorized for the '%s' collection.", c.formatCollectionName(collectionName))
	case status == http.StatusBadRequest:
		message = fmt.Sprintf(
			"The server rejected the request (HTTP 400, '%s'): check the CollectionName (required for keys allowed on multiple collections) or Event fields.",
			c.formatCollectionName(collectionName),
		)
	default:
		message = fmt.Sprintf("The server returned an error (HTTP %d).", status)
	}

	errorBody := c.readErrorBody(response)
	if errorBody != "" {
		message += fmt.Sprintf(" Server response: %s", errorBody)
	}

	if c.shouldLogHTTPFailure() {
		log.Printf("pinqloq: batch send rejected (HTTP %d, collection '%s'); logs in this group were dropped. %s", status, c.formatCollectionName(collectionName), message)
	}

	return &LogError{Reason: reason, StatusCode: status, Message: message}
}

func (c *ingestAPIClient) buildExceptionError(err error, timeout bool) *LogError {
	if timeout {
		return &LogError{Reason: LogFailureReasonTimeout, Message: "The request timed out.", Cause: err}
	}
	return &LogError{Reason: LogFailureReasonNetwork, Message: fmt.Sprintf("Network error: %v", err), Cause: err}
}

func (c *ingestAPIClient) readErrorBody(response *http.Response) string {
	limited := io.LimitReader(response.Body, maxErrorBodyCharacters+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		log.Printf("pinqloq: could not read the error response body; continuing without it. %v", err)
		return ""
	}

	body := strings.TrimSpace(string(data))
	if len(body) > maxErrorBodyCharacters {
		body = body[:maxErrorBodyCharacters]
	}
	return body
}

func (c *ingestAPIClient) resolveCollectionName(entry LogEntry) string {
	if strings.TrimSpace(entry.CollectionName) != "" {
		return entry.CollectionName
	}
	return c.options.APILogsCollectionName
}

func (c *ingestAPIClient) shouldLogHTTPFailure() bool {
	c.warnMu.Lock()
	defer c.warnMu.Unlock()

	now := time.Now()
	if now.Before(c.nextHTTPWarning) {
		return false
	}
	c.nextHTTPWarning = now.Add(httpWarningThrottle)
	return true
}

func (c *ingestAPIClient) formatCollectionName(collectionName string) string {
	if strings.TrimSpace(collectionName) == "" {
		return "(not resolved server-side)"
	}
	return collectionName
}

func (c *ingestAPIClient) toWireItem(entry LogEntry) wireLogItem {
	appVersionName := entry.AppVersionName
	if strings.TrimSpace(appVersionName) == "" {
		appVersionName = c.options.AppVersionName
	}

	deviceIdentifier := entry.DeviceIdentifier
	if strings.TrimSpace(deviceIdentifier) == "" {
		deviceIdentifier = c.options.DeviceIdentifier
	}

	var date string
	if !entry.Date.IsZero() {
		date = entry.Date.UTC().Format("2006-01-02T15:04:05.000Z")
	}

	return wireLogItem{
		LogLevel:         int(entry.LogLevel),
		Event:            entry.Event,
		Date:             date,
		AppVersionName:   appVersionName,
		DeviceIdentifier: deviceIdentifier,
		LogSourceType:    entry.LogSourceType.wireName(),
		CorrelationID:    entry.CorrelationID,
		Path:             entry.Path,
		Metadata:         entry.Metadata,
		Detail:           entry.Detail,
	}
}
