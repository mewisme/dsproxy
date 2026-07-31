package proxy

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dsproxy/internal/log"
)

// sensitiveHeaders lists header names whose values are redacted in traces.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"x-api-key":           true,
	"api-key":             true,
}

// redactHeaderValues returns a copy of h with sensitive header values replaced
// by "[redacted]".
func redactHeaderValues(h map[string]string) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if sensitiveHeaders[strings.ToLower(k)] {
			out[k] = "[redacted]"
		} else {
			out[k] = v
		}
	}
	return out
}

// headersToMap converts http.Header to a plain map[string]string, taking the
// first value for each key.
func headersToMap(h map[string][]string) map[string]string {
	m := make(map[string]string, len(h))
	for k, vv := range h {
		if len(vv) > 0 {
			m[k] = vv[0]
		}
	}
	return m
}

// TraceRequestInfo holds request-level data captured in a trace file.
type TraceRequestInfo struct {
	Method  string            `json:"method"`
	URL     string            `json:"url,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

// TraceEntry is the top-level structure written to a trace JSON file.
type TraceEntry struct {
	Timestamp      string           `json:"timestamp"`
	Client         TraceRequestInfo `json:"client"`
	Upstream       TraceRequestInfo `json:"upstream"`
	ResponseStatus int              `json:"response_status,omitempty"`
}

// writeTrace atomically writes a JSON trace file under dir. It creates the
// directory with mode 0700 and the file with mode 0600. Failures are logged
// but never propagated — tracing must not break request handling.
func writeTrace(dir string, entry TraceEntry) {
	if dir == "" {
		return
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Warn("trace: cannot create directory", "dir", dir, "err", err)
		return
	}

	filename := traceFilename()
	tmpPath := filepath.Join(dir, "."+filename+".tmp")
	finalPath := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		log.Warn("trace: marshal failed", "err", err)
		return
	}

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.Warn("trace: write failed", "path", tmpPath, "err", err)
		return
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		log.Warn("trace: rename failed", "from", tmpPath, "to", finalPath, "err", err)
		return
	}
}

func traceFilename() string {
	now := time.Now().UTC()
	ts := now.Format("20060102T150405") + fmt.Sprintf(".%09dZ", now.Nanosecond())
	suffix := randomHex(8)
	return fmt.Sprintf("%s_%s.json", ts, suffix)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fall back to a timestamp-based suffix; extremely unlikely.
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)[:n]
}
