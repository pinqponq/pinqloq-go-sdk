# Pinqloq (Go)

Pinqloq is a structured logging and log shipping SDK for centralized application logs. It
captures HTTP request/response logs through standard `net/http` middleware and sends manual
application events to the Pinqloq log management platform using in-memory buffering, batching,
and HTTPS delivery. This is the Go counterpart of the [.NET](https://www.nuget.org/packages/pinqloq),
[Node.js](https://www.npmjs.com/package/pinqloq), and [Ruby](https://rubygems.org/gems/pinqloq)
`pinqloq` SDKs — same platform, same wire protocol, idiomatic API on each side.

Because the middleware is plain `func(http.Handler) http.Handler`, it works with any router built
on `net/http.Handler` — chi, gorilla/mux, `http.ServeMux`, or a framework's own `http.Handler`
adapter (e.g. Gin's `gin.WrapH`).

## Features

- Automatic HTTP request/response logging via standard `net/http` middleware
- Correlation id read from the caller's header, falling back to a generated UUID
- Name-based redaction of sensitive fields, headers, and whole endpoints
- Manual structured application events
- Buffered and batched HTTPS delivery, backed by a single background goroutine and a Go channel
- Graceful shutdown flush, `context`-aware

## Requirements

- Go 1.22 or later
- A Pinqloq project and secret key

## Installation

```bash
go get github.com/pinqponq/pinqloq-go-sdk
```

## Quick Start

Store your secret key in an environment variable or a secret manager. Do not hardcode production
credentials.

```go
package main

import (
	"context"
	"net/http"
	"os"

	pinqloq "github.com/pinqponq/pinqloq-go-sdk"
)

func main() {
	client, err := pinqloq.New(pinqloq.Options{
		SecretKey:             os.Getenv("PINQLOQ_SECRET_KEY"),
		APILogsCollectionName: "myapp_api_logs",
		DeviceIdentifier:      "myapp-instance-1",
	})
	if err != nil {
		panic(err)
	}
	defer client.Shutdown(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/orders", ordersHandler)

	middleware := client.RequestLogging(pinqloq.RequestLoggingOptions{
		ExcludePaths: []string{"/health"},
	})

	http.ListenAndServe(":8080", middleware(mux))
}
```

The middleware captures the HTTP method, path, and status code as searchable metadata. The
request body, response body, request headers, and response headers go to the log detail as
`InputJson`, `OutputJson`, `RequestHeaders`, and `ResponseHeaders`. Bodies are truncated at 32 KB.

## Manual Logging

Call `Enqueue` directly on the client to send structured application events:

```go
client.Enqueue(pinqloq.LogEntry{
	Event:            "order.created",
	DeviceIdentifier: order.CustomerID,
	LogLevel:         pinqloq.LogLevelInformation,
	LogSourceType:    pinqloq.LogSourceTypeBackend,
	Metadata:         map[string]string{"orderId": order.ID},
}, nil, nil)
```

`client.Logger()` still returns the same `Logger` interface — useful when you want to pass just
the logging capability into a function or struct without handing it the whole client (middleware,
shutdown, and all).

`Event` and `DeviceIdentifier` are required on every entry. Leave `DeviceIdentifier` unset on an
entry to inherit the global `Options.DeviceIdentifier`. `Enqueue` returns an error if an entry has
no `DeviceIdentifier` and no global fallback is set — a missing required field fails loudly rather
than being silently dropped.

## Add Request Metadata

By default the middleware reads the required `DeviceIdentifier` from the `Device-Identifier`
request header automatically. Override how it is resolved with `ResolveDeviceIdentifier`; the
override wins, and if it returns an empty string the middleware falls back to the
`Device-Identifier` header, then to the global `Options.DeviceIdentifier`. If none of these
resolve a value, the middleware rejects the request with **HTTP 400** before it runs.

```go
middleware := client.RequestLogging(pinqloq.RequestLoggingOptions{
	ExcludePaths: []string{"/health"},
	ResolveDeviceIdentifier: func(r *http.Request) string {
		return r.Header.Get("X-User-Id")
	},
	Metadata: map[string]pinqloq.EnricherFunc{
		"userId": func(r *http.Request, statusCode int, headers http.Header) string {
			return r.Header.Get("X-User-Id")
		},
	},
})
```

Use `Metadata` for searchable values such as user and tenant IDs. Use `Detail` for additional
drill-down information. The `event` key (the panel title) defaults to `"{method} {path}"` and can
be overridden via a `Metadata["event"]` enricher.

## Correlation ID

Every log carries a `CorrelationID` that ties together the records of a single request or flow.
The request-logging middleware fills it with no configuration: the caller's `Correlation-Id`
request header when present, otherwise a generated UUID.

```go
client.Enqueue(pinqloq.LogEntry{
	Event:            "order.created",
	DeviceIdentifier: order.CustomerID,
	CorrelationID:    currentCorrelationID,
}, nil, nil)
```

## Redacting Sensitive Values

Request and response bodies and headers may contain credentials, tokens, or personal information.
Unlike the .NET SDK's attribute-based redaction (which relies on C# reflection over typed DTOs —
not available in Go's `interface{}`-based JSON decoding), this SDK redacts by **name**, exactly
like the Node.js and Ruby SDKs:

- `RedactFields` — case-insensitive field/header names masked with `*****REDACTED*****` wherever
  they appear in a captured body or header, at any nesting depth.
- `RedactPaths` — path prefixes (matched the same way as `ExcludePaths`) where every value in
  `InputJson`, `OutputJson`, `RequestHeaders`, and `ResponseHeaders` is masked, keeping the JSON
  structure and header names intact.

```go
middleware := client.RequestLogging(pinqloq.RequestLoggingOptions{
	RedactFields: []string{"ssnLastFour"},
	RedactPaths:  []string{"/payment"},
})
```

A built-in, unconditional floor of common credential names (password, token, `Authorization`,
card numbers, ...) is always masked, even with no configuration — see
[`redaction.go`](redaction.go) for the full list. `pinqloq.NewRedactionPlan` and
`pinqloq.ApplyBodyRedaction`/`pinqloq.SerializeHeaders` are exported for a project running its own
request-logging middleware that wants the same redaction behavior.

## Security and Reliability

Logs are buffered in memory and sent in batches by a single background goroutine. Buffered logs
may be lost if the process is terminated without a graceful shutdown — call `client.Shutdown(ctx)`
on exit, passing a context with a deadline generous enough for the final flush.

Delivery failures are reported through `onFailed` callbacks and, even without callbacks, as
throttled log lines via the standard `log` package — never silently discarded, but also never
blocking. If your secret key is authorized for more than one collection, set
`APILogsCollectionName` (or a per-entry `CollectionName`); otherwise the batch is rejected.

Reading a request body larger than 32 MB is capped (`io.LimitReader`) before restoring it for the
next handler, to bound memory use on an unexpectedly large upload; only the first 32 KB of that
buffer is ever sent to Pinqloq.

## Documentation

- [.NET SDK](https://www.nuget.org/packages/pinqloq), [Node.js SDK](https://www.npmjs.com/package/pinqloq),
  [Ruby SDK](https://rubygems.org/gems/pinqloq) — the other implementations of this platform's
  wire protocol and feature set.
- [Full documentation](https://pinqloq.pinqponq.io/documentation.html)

## License

MIT
