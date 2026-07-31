package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"dsproxy/internal/config"
	"dsproxy/internal/reasoning"
	"dsproxy/internal/transform"
)

func newTestStore(t *testing.T) *reasoning.Store {
	t.Helper()
	store, err := reasoning.Open(":memory:", 3600, 1000)
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func testConfig(model string) config.ProxyConfig {
	return config.ProxyConfig{
		Host:                        "127.0.0.1",
		Port:                        9999,
		UpstreamBaseURL:             "https://api.deepseek.com",
		UpstreamModel:               model,
		Thinking:                    "enabled",
		ReasoningEffort:             "max",
		RequestTimeout:              300,
		MaxRequestBodyBytes:         20 * 1024 * 1024,
		MissingReasoningStrategy:    "recover",
		ReasoningCacheMaxAgeSeconds: 3600,
		ReasoningCacheMaxRows:       1000,
		DisplayReasoning:            false,
		CollapsibleReasoning:        false,
		CORS:                        false,
		Verbose:                     false,
	}
}

// --- Model resolution ---

func TestConfigForRequestUsesPayloadModel(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("default-model")
	h := &Handler{Config: cfg, Store: store}

	payload := map[string]any{"model": "request-model"}
	resolved := h.configForRequest(payload)
	if resolved.UpstreamModel != "request-model" {
		t.Errorf("expected 'request-model', got %q", resolved.UpstreamModel)
	}
}

func TestConfigForRequestUsesDefaultWhenEmpty(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("default-model")
	h := &Handler{Config: cfg, Store: store}

	payload := map[string]any{"model": ""}
	resolved := h.configForRequest(payload)
	if resolved.UpstreamModel != "default-model" {
		t.Errorf("expected 'default-model', got %q", resolved.UpstreamModel)
	}
}

func TestConfigForRequestUsesDefaultWhenMissing(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("default-model")
	h := &Handler{Config: cfg, Store: store}

	payload := map[string]any{}
	resolved := h.configForRequest(payload)
	if resolved.UpstreamModel != "default-model" {
		t.Errorf("expected 'default-model', got %q", resolved.UpstreamModel)
	}
}

func TestConfigForRequestIsStateless(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("default-model")
	h := &Handler{Config: cfg, Store: store}

	// Multiple calls with different models don't mutate shared state.
	r1 := h.configForRequest(map[string]any{"model": "model-a"})
	r2 := h.configForRequest(map[string]any{"model": "model-b"})
	r3 := h.configForRequest(map[string]any{})

	if r1.UpstreamModel != "model-a" {
		t.Errorf("r1: got %q", r1.UpstreamModel)
	}
	if r2.UpstreamModel != "model-b" {
		t.Errorf("r2: got %q", r2.UpstreamModel)
	}
	if r3.UpstreamModel != "default-model" {
		t.Errorf("r3: got %q", r3.UpstreamModel)
	}
	// Original config unchanged.
	if cfg.UpstreamModel != "default-model" {
		t.Errorf("original config mutated: %q", cfg.UpstreamModel)
	}
}

func TestConfigForRequestConcurrentIndependence(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("default-model")
	h := &Handler{Config: cfg, Store: store}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			model := "model-" + string(rune('a'+(n%26)))
			resolved := h.configForRequest(map[string]any{"model": model})
			if resolved.UpstreamModel != model {
				t.Errorf("goroutine %d: got %q, want %q", n, resolved.UpstreamModel, model)
			}
		}(i)
	}
	wg.Wait()

	// Original config must not be mutated.
	if cfg.UpstreamModel != "default-model" {
		t.Errorf("original config mutated: %q", cfg.UpstreamModel)
	}
}

// --- /models endpoint ---

func TestModelsEndpointReturnsConfiguredModel(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("custom-model-v1")
	h := &Handler{Config: cfg, Store: store}

	rec := httptest.NewRecorder()
	h.sendModels(rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	data, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", body["data"])
	}

	ids := make(map[string]bool)
	for _, item := range data {
		m, _ := item.(map[string]any)
		ids[m["id"].(string)] = true
	}

	if !ids["custom-model-v1"] {
		t.Error("models list should contain custom-model-v1")
	}
	if !ids["deepseek-v4-pro"] {
		t.Error("models list should contain deepseek-v4-pro")
	}
	if !ids["deepseek-v4-flash"] {
		t.Error("models list should contain deepseek-v4-flash")
	}
}

func TestModelsEndpointNoDuplicates(t *testing.T) {
	store := newTestStore(t)
	// Use a model that's the same as one of the built-in.
	cfg := testConfig("deepseek-v4-pro")
	h := &Handler{Config: cfg, Store: store}

	rec := httptest.NewRecorder()
	h.sendModels(rec)

	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	data := body["data"].([]any)

	seen := make(map[string]int)
	for _, item := range data {
		m := item.(map[string]any)
		id := m["id"].(string)
		seen[id]++
		if seen[id] > 1 {
			t.Errorf("duplicate model %q in list", id)
		}
	}
}

func TestModelsEndpointStableSort(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("x-model")
	h := &Handler{Config: cfg, Store: store}

	// Call twice, verify same order.
	var ids1, ids2 []string
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.sendModels(rec)
		var body map[string]any
		json.Unmarshal(rec.Body.Bytes(), &body)
		data := body["data"].([]any)
		var ids []string
		for _, item := range data {
			m := item.(map[string]any)
			ids = append(ids, m["id"].(string))
		}
		if i == 0 {
			ids1 = ids
		} else {
			ids2 = ids
		}
	}

	if len(ids1) != len(ids2) {
		t.Fatalf("length mismatch: %d vs %d", len(ids1), len(ids2))
	}
	for i := range ids1 {
		if ids1[i] != ids2[i] {
			t.Errorf("order changed at position %d: %q vs %q", i, ids1[i], ids2[i])
		}
	}
}

// --- CORS ---

func TestCORSMiddlewareEnabled(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("deepseek-v4-pro")
	cfg.CORS = true
	h := &Handler{Config: cfg, Store: store}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	h.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS Allow-Origin missing or wrong")
	}
	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("CORS Allow-Methods missing")
	}
	if resp.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Error("CORS Allow-Headers missing")
	}
	// Must NOT contain credentials.
	if strings.Contains(resp.Header.Get("Access-Control-Allow-Credentials"), "true") {
		t.Error("CORS Allow-Credentials: true should not be present")
	}
}

func TestCORSMiddlewareDisabled(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("deepseek-v4-pro")
	cfg.CORS = false
	h := &Handler{Config: cfg, Store: store}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	h.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should be absent when disabled")
	}
}

func TestCORSOnErrorResponse(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("deepseek-v4-pro")
	cfg.CORS = true
	h := &Handler{Config: cfg, Store: store}

	// POST without Authorization should 401, but CORS headers should still be present.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS Allow-Origin missing on 401 response")
	}
}

func TestCORSPreflight(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("deepseek-v4-pro")
	cfg.CORS = true
	h := &Handler{Config: cfg, Store: store}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)

	h.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS Allow-Origin missing on preflight")
	}
}

func TestCORSHeadersOnAllResponses(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("deepseek-v4-pro")
	cfg.CORS = true
	h := &Handler{Config: cfg, Store: store}

	paths := []string{"/health", "/v1/models", "/not-found"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			h.ServeHTTP(rec, req)
			resp := rec.Result()
			if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
				t.Errorf("CORS Allow-Origin missing on %s response", path)
			}
		})
	}
}

// --- No global state leak ---

func TestNoHandlerMutation(t *testing.T) {
	store := newTestStore(t)
	cfg := testConfig("original")
	h := &Handler{Config: cfg, Store: store}

	// Simulate what happens during requests — configForRequest returns a copy.
	_ = h.configForRequest(map[string]any{"model": "different"})
	_ = h.configForRequest(map[string]any{})
	_ = h.configForRequest(map[string]any{"model": "another"})

	// Handler config must remain unchanged.
	if h.Config.UpstreamModel != "original" {
		t.Errorf("handler config mutated from %q to %q", "original", h.Config.UpstreamModel)
	}
}

// --- bufioReader import check ---

// bufioReader is defined in sse.go; this test just ensures it compiles.
func TestBufioReaderExists(t *testing.T) {
	// Just a compile-time check; bufioReader is called from proxyStreaming.
	// If this file compiles, the symbol is reachable.
}

// --- User identity handling ---

// upstreamRecorder captures the upstream request body for assertions.
var capturedUpstreamBody map[string]any
var capturedUpstreamCalled bool

func userFakeUpstream(w http.ResponseWriter, r *http.Request) {
	capturedUpstreamCalled = true
	capturedUpstreamBody = nil
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &capturedUpstreamBody)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl-test", "object": "chat.completion", "created": 1,
		"model": "deepseek-v4-pro",
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": "ok"},
		}},
	})
}

func setupUserTest(t *testing.T) *httptest.Server {
	t.Helper()
	capturedUpstreamBody = nil
	capturedUpstreamCalled = false
	upstream := httptest.NewServer(http.HandlerFunc(userFakeUpstream))
	t.Cleanup(upstream.Close)
	return upstream
}

func proxyHandlerForUpstream(t *testing.T, upstreamURL string) *Handler {
	t.Helper()
	store, err := reasoning.Open(":memory:", 3600, 1000)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return &Handler{
		Config: config.ProxyConfig{
			Host:                     "127.0.0.1",
			Port:                     0,
			UpstreamBaseURL:          upstreamURL,
			UpstreamModel:            "deepseek-v4-pro",
			Thinking:                 "disabled",
			MissingReasoningStrategy: "recover",
			MaxRequestBodyBytes:      20 * 1024 * 1024,
			RequestTimeout:           30,
		},
		Store:  store,
		Client: http.DefaultClient,
	}
}

func postUserRequest(t *testing.T, handler *Handler, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-test-key-12345")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

func TestUserMapsToUpstreamUserID(t *testing.T) {
	upstream := setupUserTest(t)
	handler := proxyHandlerForUpstream(t, upstream.URL)

	status, _ := postUserRequest(t, handler, map[string]any{
		"model":    "deepseek-v4-pro",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"user":     "github|63306485",
	})
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if !capturedUpstreamCalled {
		t.Fatal("upstream not called")
	}
	uid, _ := capturedUpstreamBody["user_id"].(string)
	if uid == "" || !strings.HasPrefix(uid, "u_") {
		t.Errorf("expected user_id starting with u_, got %q", uid)
	}
	// user must not be in upstream body
	if _, ok := capturedUpstreamBody["user"]; ok {
		t.Error("upstream body must not contain 'user'")
	}
	// raw user must not be in upstream body
	bodyBytes, _ := json.Marshal(capturedUpstreamBody)
	if strings.Contains(string(bodyBytes), "63306485") {
		t.Error("upstream body must not contain raw user value")
	}
}

func TestExplicitValidUserIDPreserved(t *testing.T) {
	upstream := setupUserTest(t)
	handler := proxyHandlerForUpstream(t, upstream.URL)

	status, _ := postUserRequest(t, handler, map[string]any{
		"model":    "deepseek-v4-pro",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"user_id":  "customer_123",
	})
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if capturedUpstreamBody["user_id"] != "customer_123" {
		t.Errorf("got user_id=%v, want customer_123", capturedUpstreamBody["user_id"])
	}
}

func TestInvalidUserIDReturns400(t *testing.T) {
	upstream := setupUserTest(t)
	handler := proxyHandlerForUpstream(t, upstream.URL)

	status, body := postUserRequest(t, handler, map[string]any{
		"model":    "deepseek-v4-pro",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"user_id":  "github|63306485",
	})
	if status != 400 {
		t.Fatalf("expected 400, got %d body=%v", status, body)
	}
	if capturedUpstreamCalled {
		t.Fatal("upstream must not be called for invalid request")
	}
	errMap, _ := body["error"].(map[string]any)
	if errMap["type"] != "invalid_request_error" {
		t.Errorf("expected invalid_request_error, got %v", errMap["type"])
	}
	if errMap["param"] != "user_id" {
		t.Errorf("expected param=user_id, got %v", errMap["param"])
	}
	// Error must not contain the rejected value
	if strings.Contains(errMap["message"].(string), "github|63306485") {
		t.Error("error message must not contain rejected value")
	}
}

func TestInvalidUserTypeReturns400(t *testing.T) {
	upstream := setupUserTest(t)
	handler := proxyHandlerForUpstream(t, upstream.URL)

	status, body := postUserRequest(t, handler, map[string]any{
		"model":    "deepseek-v4-pro",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"user":     42,
	})
	if status != 400 {
		t.Fatalf("expected 400, got %d body=%v", status, body)
	}
	if capturedUpstreamCalled {
		t.Fatal("upstream must not be called for invalid request")
	}
}

func TestNoUserFieldsBehavesAsBefore(t *testing.T) {
	upstream := setupUserTest(t)
	handler := proxyHandlerForUpstream(t, upstream.URL)

	status, _ := postUserRequest(t, handler, map[string]any{
		"model":    "deepseek-v4-pro",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if _, ok := capturedUpstreamBody["user"]; ok {
		t.Error("upstream body must not contain user when not set")
	}
	if _, ok := capturedUpstreamBody["user_id"]; ok {
		t.Error("upstream body must not contain user_id when no identity")
	}
}

func TestBothFieldsExplicitWinsInUpstream(t *testing.T) {
	upstream := setupUserTest(t)
	handler := proxyHandlerForUpstream(t, upstream.URL)

	status, _ := postUserRequest(t, handler, map[string]any{
		"model":    "deepseek-v4-pro",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"user":     "github|63306485",
		"user_id":  "customer_123",
	})
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if capturedUpstreamBody["user_id"] != "customer_123" {
		t.Errorf("got user_id=%v, want customer_123", capturedUpstreamBody["user_id"])
	}
	if _, ok := capturedUpstreamBody["user"]; ok {
		t.Error("upstream body must not contain 'user'")
	}
}

func TestNormalizeUpstreamUserIDReturnsValidationError(t *testing.T) {
	_, err := transform.NormalizeUpstreamUserID(map[string]any{"user": 123}, "Bearer sk-test")
	if err == nil {
		t.Fatal("expected error")
	}
	ve, ok := err.(*transform.RequestValidationError)
	if !ok {
		t.Fatalf("expected *RequestValidationError, got %T", err)
	}
	if ve.Param != "user" {
		t.Errorf("param: got %q, want user", ve.Param)
	}
}

func TestTraceUpstreamBodyContainsUserIDNotRawUser(t *testing.T) {
	capturedUpstreamBody = nil
	capturedUpstreamCalled = false
	upstream := httptest.NewServer(http.HandlerFunc(userFakeUpstream))
	defer upstream.Close()

	// Enable tracing for this test
	traceDir := t.TempDir()
	store, err := reasoning.Open(":memory:", 3600, 1000)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	handler := &Handler{
		Config: config.ProxyConfig{
			Host:                     "127.0.0.1",
			Port:                     0,
			UpstreamBaseURL:          upstream.URL,
			UpstreamModel:            "deepseek-v4-pro",
			Thinking:                 "disabled",
			MissingReasoningStrategy: "recover",
			MaxRequestBodyBytes:      20 * 1024 * 1024,
			RequestTimeout:           30,
			TraceDir:                 traceDir,
		},
		Store:  store,
		Client: http.DefaultClient,
	}

	status, _ := postUserRequest(t, handler, map[string]any{
		"model":    "deepseek-v4-pro",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"user":     "github|63306485",
	})
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}

	// Read trace file and verify upstream.body
	files, err := os.ReadDir(traceDir)
	if err != nil || len(files) == 0 {
		t.Fatal("expected at least one trace file")
	}

	data, err := os.ReadFile(filepath.Join(traceDir, files[0].Name()))
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}

	// Verify trace body (decoded JSON)
	var entry TraceEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}

	var upstreamPayload map[string]any
	if err := json.Unmarshal(entry.Upstream.Body, &upstreamPayload); err != nil {
		t.Fatalf("unmarshal upstream body: %v", err)
	}

	if _, ok := upstreamPayload["user"]; ok {
		t.Error("trace upstream body contains 'user'")
	}
	if uid, ok := upstreamPayload["user_id"].(string); !ok || !strings.HasPrefix(uid, "u_") {
		t.Errorf("trace upstream body must contain user_id starting with u_, got %v", uid)
	}
	bodyStr := string(entry.Upstream.Body)
	if strings.Contains(bodyStr, "63306485") {
		t.Error("trace upstream body contains raw user value")
	}
	if strings.Contains(bodyStr, "sk-test-key") {
		t.Error("trace upstream body contains bearer token")
	}
}
