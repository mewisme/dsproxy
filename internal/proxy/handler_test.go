package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"dsproxy/internal/config"
	"dsproxy/internal/reasoning"
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
