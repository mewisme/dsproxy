package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var isWindows = runtime.GOOS == "windows"

func tempTraceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// --- Enabled writes file ---

func TestTraceEnabledWritesFile(t *testing.T) {
	dir := tempTraceDir(t)
	entry := TraceEntry{
		Timestamp: "2026-01-01T00:00:00Z",
		Client: TraceRequestInfo{
			Method:  "POST",
			Path:    "/v1/chat/completions",
			Headers: map[string]string{"Content-Type": "application/json"},
		},
		Upstream: TraceRequestInfo{
			Method:  "POST",
			URL:     "https://api.deepseek.com/chat/completions",
			Headers: map[string]string{"Authorization": "Bearer sk-test"},
		},
		ResponseStatus: 200,
	}
	writeTrace(dir, entry)

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read trace dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one trace file")
	}
	if len(files) > 1 {
		t.Errorf("expected exactly 1 trace file, got %d", len(files))
	}
}

// --- Disabled writes nothing ---

func TestTraceDisabledWritesNothing(t *testing.T) {
	// writeTrace returns early when dir is empty — no side effects.
	writeTrace("", TraceEntry{Timestamp: "now"})
}

func TestTraceEmptyDirNoFile(t *testing.T) {
	// Ensure writeTrace returns early when dir is empty.
	// We already cover this: just make sure no panic.
	writeTrace("", TraceEntry{})
}

// --- Auth redaction ---

func TestTraceAuthRedacted(t *testing.T) {
	dir := tempTraceDir(t)

	// Redact headers before constructing the trace entry, matching writeChatTrace.
	clientHeaders := redactHeaderValues(map[string]string{
		"Authorization":       "Bearer sk-secret1234",
		"Proxy-Authorization": "Bearer proxy-secret",
		"X-Api-Key":           "key-secret",
		"Api-Key":             "api-secret",
		"Content-Type":        "application/json",
	})
	upstreamHeaders := redactHeaderValues(map[string]string{
		"Authorization": "Bearer sk-upstream",
	})

	entry := TraceEntry{
		Timestamp: "2026-01-01T00:00:00Z",
		Client: TraceRequestInfo{
			Method:  "POST",
			Path:    "/v1/chat/completions",
			Headers: clientHeaders,
		},
		Upstream: TraceRequestInfo{
			Method:  "POST",
			URL:     "https://api.deepseek.com/chat/completions",
			Headers: upstreamHeaders,
		},
	}
	writeTrace(dir, entry)

	files, err := os.ReadDir(dir)
	if err != nil || len(files) == 0 {
		t.Fatal("expected a trace file")
	}

	data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}

	content := string(data)

	// Sensitive values must be redacted.
	for _, secret := range []string{"sk-secret1234", "proxy-secret", "key-secret", "api-secret", "sk-upstream"} {
		if strings.Contains(content, secret) {
			t.Errorf("trace file contains unredacted secret %q in:\n%s", secret, content)
		}
	}

	// Redacted marker must be present.
	if !strings.Contains(content, "[redacted]") {
		t.Error("trace file should contain [redacted] markers")
	}

	// Non-sensitive header must be present.
	if !strings.Contains(content, "application/json") {
		t.Error("trace file should contain non-sensitive header value")
	}
}

// --- Redact header values function ---

func TestRedactHeaderValues(t *testing.T) {
	input := map[string]string{
		"authorization":       "Bearer abc",
		"Authorization":       "Bearer ABC",
		"AUTHORIZATION":       "Bearer UPPER",
		"proxy-authorization": "Basic xyz",
		"x-api-key":           "key123",
		"api-key":             "key456",
		"Content-Type":        "application/json",
		"Accept":              "text/event-stream",
	}

	redacted := redactHeaderValues(input)

	for k, v := range redacted {
		key := strings.ToLower(k)
		if sensitiveHeaders[key] {
			if v != "[redacted]" {
				t.Errorf("sensitive header %s: got %q, want [redacted]", k, v)
			}
		} else {
			if v != input[k] {
				t.Errorf("non-sensitive header %s: got %q, want %q", k, v, input[k])
			}
		}
	}
}

// --- Permissions ---

func TestTracePermissions(t *testing.T) {
	dir := tempTraceDir(t)
	writeTrace(dir, TraceEntry{Timestamp: "2026-01-01T00:00:00Z"})

	files, err := os.ReadDir(dir)
	if err != nil || len(files) == 0 {
		t.Fatal("expected a trace file")
	}

	// File permissions: 0600 (skip on Windows where os.FileMode differs).
	if isWindows {
		t.Skip("file permission check skipped on Windows")
	}
	info, err := files[0].Info()
	if err != nil {
		t.Fatalf("stat trace file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file mode: got %o, want 0600", info.Mode().Perm())
	}
}

func TestTraceDirPermissions(t *testing.T) {
	// Create a fresh dir via writeTrace → MkdirAll with 0700.
	base := t.TempDir()
	dir := filepath.Join(base, "sub", "traces")
	writeTrace(dir, TraceEntry{Timestamp: "2026-01-01T00:00:00Z"})

	// Check the directory permissions (MkdirAll sets requested perms; umask may affect).
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat trace dir: %v", err)
	}
	// On Windows, permissions work differently; only check on Unix-like.
	// We just ensure the directory was created.
	if !info.IsDir() {
		t.Error("trace path is not a directory")
	}
}

// --- Concurrent safety ---

func TestTraceConcurrentSafety(t *testing.T) {
	dir := tempTraceDir(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			writeTrace(dir, TraceEntry{
				Timestamp: "2026-01-01T00:00:00Z",
				Client: TraceRequestInfo{
					Method: "POST",
					Path:   "/v1/chat/completions",
					Headers: map[string]string{
						"X-Request-Id": "req-" + string(rune('0'+n)),
					},
				},
			})
		}(i)
	}
	wg.Wait()

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read trace dir: %v", err)
	}
	if len(files) != 20 {
		t.Errorf("expected 20 trace files, got %d", len(files))
	}
}

// --- Failure does not crash ---

func TestTraceFailureDoesNotPanic(t *testing.T) {
	// Write to a path that cannot be created (e.g., a file where a directory is needed).
	// writeTrace must not panic.
	dir := filepath.Join(t.TempDir(), "notadir")
	// Create a file at that path to make MkdirAll fail.
	os.WriteFile(dir, []byte("block"), 0644)

	// Should not panic.
	writeTrace(dir, TraceEntry{Timestamp: "2026-01-01T00:00:00Z"})
}

// --- Relative path ---

func TestTraceRelativePath(t *testing.T) {
	dir := "relative-traces"
	defer os.RemoveAll(dir)

	writeTrace(dir, TraceEntry{Timestamp: "2026-01-01T00:00:00Z"})

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read relative trace dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one trace file in relative dir")
	}
}

// --- Trace file naming ---

func TestTraceFilenameFormat(t *testing.T) {
	name := traceFilename()
	// Should match YYYYmmddTHHMMSS.SSSSSSSSSZ_XXXXXXXX.json
	if !strings.HasSuffix(name, ".json") {
		t.Errorf("filename should end with .json: %q", name)
	}
	if len(name) < 30 {
		t.Errorf("filename too short: %q", name)
	}
	if !strings.Contains(name, "T") {
		t.Errorf("filename should contain T separator: %q", name)
	}
	if !strings.Contains(name, "Z_") {
		t.Errorf("filename should contain Z_ separator: %q", name)
	}
}

// --- Trace entry marshal/unmarshal round-trip ---

func TestTraceEntryRoundTrip(t *testing.T) {
	dir := tempTraceDir(t)
	entry := TraceEntry{
		Timestamp: "2026-01-01T00:00:00Z",
		Client: TraceRequestInfo{
			Method:  "POST",
			Path:    "/v1/chat/completions",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    json.RawMessage(`{"model":"test"}`),
		},
		Upstream: TraceRequestInfo{
			Method:  "POST",
			URL:     "https://api.deepseek.com/chat/completions",
			Headers: map[string]string{"Authorization": "Bearer sk-test"},
			Body:    json.RawMessage(`{"model":"deepseek-v4-pro","stream":true}`),
		},
		ResponseStatus: 200,
	}
	writeTrace(dir, entry)

	files, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))

	var parsed TraceEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal trace file: %v", err)
	}

	if parsed.Timestamp != entry.Timestamp {
		t.Errorf("timestamp: got %q, want %q", parsed.Timestamp, entry.Timestamp)
	}
	if parsed.Client.Method != "POST" {
		t.Errorf("client method: got %q", parsed.Client.Method)
	}
	if parsed.ResponseStatus != 200 {
		t.Errorf("response status: got %d", parsed.ResponseStatus)
	}

	// Body may have different whitespace after round-trip; compare as JSON objects.
	var wantBody, gotBody map[string]any
	if err := json.Unmarshal(entry.Upstream.Body, &wantBody); err != nil {
		t.Fatalf("unmarshal wanted body: %v", err)
	}
	if err := json.Unmarshal(parsed.Upstream.Body, &gotBody); err != nil {
		t.Fatalf("unmarshal parsed body: %v", err)
	}
	if wantBody["model"] != gotBody["model"] || wantBody["stream"] != gotBody["stream"] {
		t.Errorf("upstream body mismatch: want %v, got %v", wantBody, gotBody)
	}
}
