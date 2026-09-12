package pinqloq

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxBodyCharacters = 32 * 1024
	maxBodyBytes      = 4 * maxBodyCharacters
	maxRestoreBytes   = 32 * 1024 * 1024
)

const (
	deviceIdentifierHeaderName = "Device-Identifier"
	correlationIDHeaderName    = "Correlation-Id"
)

const deviceIdentifierRequiredMessage = "pinqloq: the required DeviceIdentifier could not be resolved. Send the " +
	"'Device-Identifier' request header, or configure ResolveDeviceIdentifier, or set Options.DeviceIdentifier."

const (
	serverErrorStatusThreshold = 500
	clientErrorStatusThreshold = 400
)

const selectorWarningThrottle = 60 * time.Second

// EnricherFunc computes a metadata/detail value from the request and the completed response.
type EnricherFunc func(r *http.Request, statusCode int, responseHeaders http.Header) string

// RequestLoggingOptions configures Client.RequestLogging.
type RequestLoggingOptions struct {
	ExcludePaths            []string
	ResolveDeviceIdentifier func(r *http.Request) string
	ResolveAppVersionName   func(r *http.Request) string
	Metadata                map[string]EnricherFunc
	Detail                  map[string]EnricherFunc
	RedactFields            []string
	RedactPaths             []string
}

// RequestLogging returns standard net/http middleware that captures every HTTP request, produces
// one API log, and enqueues it via the client's Logger. Compatible with any router built on
// net/http.Handler (chi, gorilla/mux, ServeMux, ...).
func (c *Client) RequestLogging(opts RequestLoggingOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if matchesAnyPathPrefix(r.URL.Path, opts.ExcludePaths) {
				next.ServeHTTP(w, r)
				return
			}

			deviceIdentifier := resolveDeviceIdentifier(r, opts, c.options)
			if strings.TrimSpace(deviceIdentifier) == "" {
				http.Error(w, deviceIdentifierRequiredMessage, http.StatusBadRequest)
				return
			}

			redactPlan := buildRedactPlan(r.URL.Path, opts)
			startedAt := time.Now()

			requestHeaders := truncateBody(SerializeHeaders(r.Header, redactPlan))
			inputJSON := truncateBody(ApplyBodyRedaction(readAndRestoreBody(r), redactPlan))
			correlationID := resolveCorrelationID(r)
			appVersionName := resolveSelector(opts.ResolveAppVersionName, r, "ResolveAppVersionName")

			crw := newCapturingResponseWriter(w, maxBodyBytes)
			next.ServeHTTP(crw, r)

			elapsedMs := time.Since(startedAt).Milliseconds()
			outputJSON := truncateBody(ApplyBodyRedaction(crw.capturedString(), redactPlan))
			responseHeaders := truncateBody(SerializeHeaders(crw.Header(), redactPlan))

			metadata := map[string]string{}
			metadata["event"] = strings.TrimSpace(fmt.Sprintf("%s %s", r.Method, r.URL.Path))
			applyEnrichers(metadata, opts.Metadata, r, crw.statusCode, crw.Header())
			resolvedEventName := metadata["event"]
			delete(metadata, "event")

			metadata["method"] = r.Method
			metadata["statusCode"] = strconv.Itoa(crw.statusCode)
			metadata["durationMs"] = strconv.FormatInt(elapsedMs, 10)
			metadata["RequestMethod"] = r.Method
			metadata["ResponseCode"] = strconv.Itoa(crw.statusCode)

			detail := map[string]string{}
			applyEnrichers(detail, opts.Detail, r, crw.statusCode, crw.Header())
			detail["InputJson"] = inputJSON
			detail["OutputJson"] = outputJSON
			detail["RequestHeaders"] = requestHeaders
			detail["ResponseHeaders"] = responseHeaders

			entry := LogEntry{
				LogLevel:         resolveLogLevel(crw.statusCode),
				Event:            resolvedEventName,
				DeviceIdentifier: deviceIdentifier,
				AppVersionName:   appVersionName,
				LogSourceType:    LogSourceTypeBackend,
				CorrelationID:    correlationID,
				Path:             r.URL.Path,
				Metadata:         metadata,
				Detail:           detail,
			}

			if _, err := c.logger.Enqueue(entry, nil, nil); err != nil {
				log.Printf("pinqloq: %v", err)
			}
		})
	}
}

func buildRedactPlan(path string, opts RequestLoggingOptions) *RedactionPlan {
	if len(opts.RedactPaths) > 0 && matchesAnyPathPrefix(path, opts.RedactPaths) {
		return redactionPlanAll
	}
	return NewRedactionPlan(false, opts.RedactFields)
}

func resolveLogLevel(statusCode int) LogLevel {
	if statusCode >= serverErrorStatusThreshold {
		return LogLevelError
	}
	if statusCode >= clientErrorStatusThreshold {
		return LogLevelWarning
	}
	return LogLevelInformation
}

func resolveDeviceIdentifier(r *http.Request, opts RequestLoggingOptions, globalOptions Options) string {
	if overridden := resolveSelector(opts.ResolveDeviceIdentifier, r, "ResolveDeviceIdentifier"); overridden != "" {
		return overridden
	}
	if header := strings.TrimSpace(r.Header.Get(deviceIdentifierHeaderName)); header != "" {
		return header
	}
	return globalOptions.DeviceIdentifier
}

func resolveCorrelationID(r *http.Request) string {
	if header := strings.TrimSpace(r.Header.Get(correlationIDHeaderName)); header != "" {
		return header
	}
	return generateUUID()
}

func resolveSelector(selector func(r *http.Request) string, r *http.Request, throttleKey string) (result string) {
	if selector == nil {
		return ""
	}

	defer func() {
		if rec := recover(); rec != nil {
			warnThrottled(throttleKey, selectorWarningThrottle, "pinqloq: %s panicked; ignored. %v", throttleKey, rec)
			result = ""
		}
	}()

	return strings.TrimSpace(selector(r))
}

func applyEnrichers(target map[string]string, enrichers map[string]EnricherFunc, r *http.Request, statusCode int, headers http.Header) {
	for key, selector := range enrichers {
		value := callEnricher(selector, r, statusCode, headers, key)
		if value != "" {
			target[key] = value
		}
	}
}

func callEnricher(selector EnricherFunc, r *http.Request, statusCode int, headers http.Header, key string) (result string) {
	defer func() {
		if rec := recover(); rec != nil {
			warnThrottled("enricher:"+key, selectorWarningThrottle, "pinqloq: the '%s' enricher panicked; ignored. %v", key, rec)
			result = ""
		}
	}()

	return selector(r, statusCode, headers)
}

func readAndRestoreBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}

	full, err := io.ReadAll(io.LimitReader(r.Body, maxRestoreBytes))
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(full))

	if len(full) > maxBodyBytes {
		return string(full[:maxBodyBytes])
	}
	return string(full)
}

func truncateBody(value string) string {
	if len(value) <= maxBodyCharacters {
		return value
	}
	return value[:maxBodyCharacters]
}

func generateUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
