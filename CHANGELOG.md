# Changelog

All notable changes to the `pinqloq` Go module are documented here. This module follows
[Semantic Versioning](https://semver.org). Its version numbers are independent of the .NET
NuGet package, the npm package, and the RubyGems gem — all four ship on separate cadences for
the same platform, and mirror each other's feature set rather than their version numbers.

## 2.0.0 — 2026-09-12

**Changed (breaking):**

- `Client.Middleware(RequestLoggingOptions)` renamed to `Client.RequestLogging(RequestLoggingOptions)`.
  Same signature and behavior — only the name changed, to match the concept's name in the .NET
  (`UsePinqloqRequestLogging`), Node.js (`client.requestLogging`), and Ruby
  (`Pinqloq::Rack::RequestLogging`) SDKs.
- Module path is now `github.com/pinqponq/pinqloq-go-sdk/v2`, per Go's semantic import
  versioning rules for a v2+ module. Update both the `go get` target and the import path.

## 1.1.0 — 2026-09-11

**Added:**

- `Client.Enqueue` / `Client.EnqueueMany` — shortcuts for `Client.Logger().Enqueue` /
  `Client.Logger().EnqueueMany`, so manual logging no longer needs the extra `.Logger()` call.
  `Client.Logger()` is unchanged and still useful when a function or struct should only receive
  the logging capability, not the whole client.

## 1.0.0 — 2026-09-08

**Added:**

- Initial release: feature parity with the .NET, Node.js, and Ruby SDKs' core surface, including
  the `Path` fixed field ([pinqponq/pinqloq#108](https://github.com/pinqponq/pinqloq/issues/108))
  from day one.
- `pinqloq.New(Options)` — buffered, batched manual structured logging (`Logger.Enqueue` /
  `Logger.EnqueueMany`), delivered to the same `/bulk` ingest endpoint the other SDKs use. The
  queue is a Go channel and the dispatcher a single background goroutine, batching on size or a
  `FlushInterval` timer.
- `Client.Middleware(RequestLoggingOptions)` — standard `func(http.Handler) http.Handler`
  middleware, compatible with any router built on `net/http.Handler` (chi, gorilla/mux,
  `http.ServeMux`, ...): captures method/path/status/duration, request/response bodies (32KB cap)
  and headers, resolves `DeviceIdentifier` (resolver → `Device-Identifier` header → global
  fallback, HTTP 400 if none resolve) and `CorrelationID` (`Correlation-Id` header → generated
  UUID).
- Redaction: a built-in, unconditional credential-name floor (password, token, Authorization,
  ...) plus `RedactFields` (name-based, any nesting depth) and `RedactPaths` (whole-body/header
  masking). Fails closed on a sensitive body that can't be parsed as JSON, matching the other
  three SDKs' behavior.
