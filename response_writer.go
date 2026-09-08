package pinqloq

import "net/http"

type capturingResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	wroteHeader   bool
	captured      []byte
	maxBytes      int
	capturedBytes int
}

func newCapturingResponseWriter(w http.ResponseWriter, maxBytes int) *capturingResponseWriter {
	return &capturingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK, maxBytes: maxBytes}
}

func (w *capturingResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *capturingResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	if w.capturedBytes < w.maxBytes {
		remaining := w.maxBytes - w.capturedBytes
		toCapture := data
		if len(toCapture) > remaining {
			toCapture = toCapture[:remaining]
		}
		w.captured = append(w.captured, toCapture...)
		w.capturedBytes += len(toCapture)
	}

	return w.ResponseWriter.Write(data)
}

func (w *capturingResponseWriter) capturedString() string {
	return string(w.captured)
}
