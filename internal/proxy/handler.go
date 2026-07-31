package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dsproxy/internal/config"
	"dsproxy/internal/log"
	"dsproxy/internal/reasoning"
	"dsproxy/internal/stream"
	"dsproxy/internal/transform"
)

type Handler struct {
	Config config.ProxyConfig
	Store  *reasoning.Store
	Client *http.Client
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

	// Apply CORS headers middleware-style before routing.
	if h.Config.CORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length")
	}

	h.logIncoming(r)
	defer func() {
		h.logCompleted(r, rec, started)
	}()

	path := r.URL.Path
	switch r.Method {
	case http.MethodOptions:
		h.handleOptions(rec, r, path)
		return
	case http.MethodGet:
		h.handleGet(rec, r, path)
		return
	case http.MethodPost:
		h.handlePost(rec, r, path, started)
		return
	default:
		writeJSON(rec, http.StatusMethodNotAllowed, map[string]any{
			"error": map[string]any{"message": "Method not allowed"},
		})
	}
}

func (h *Handler) handleOptions(w http.ResponseWriter, r *http.Request, path string) {
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request, path string) {
	switch path {
	case "/health", "/healthz", "/v1/health", "/v1/healthz":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "/models", "/v1/models":
		h.sendModels(w)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "Not found"}})
	}
}

// configForRequest returns a copy of the handler config with the upstream
// model resolved from the request payload. It does not mutate handler state.
func (h *Handler) configForRequest(payload map[string]any) config.ProxyConfig {
	cfg := h.Config
	if m, ok := payload["model"].(string); ok && strings.TrimSpace(m) != "" {
		cfg.UpstreamModel = transform.UpstreamModelFor(strings.TrimSpace(m), cfg)
	}
	return cfg
}

// sendModels writes a stable model list based on the configured model plus
// well-known DeepSeek models.
func (h *Handler) sendModels(w http.ResponseWriter) {
	created := time.Now().Unix()
	ids := []string{h.Config.UpstreamModel, "deepseek-v4-pro", "deepseek-v4-flash"}
	seen := map[string]struct{}{}
	var models []any
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, map[string]any{
			"id": id, "object": "model", "created": created, "owned_by": "deepseek",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request, path string, started time.Time) {
	if path != "/chat/completions" && path != "/v1/chat/completions" {
		log.Warn("rejected unsupported path", "path", path)
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]any{"message": "Only /v1/chat/completions is supported"},
		})
		return
	}
	auth := cursorAuthorization(r)
	if auth == "" {
		log.Warn("rejected missing authorization", "path", path)
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": map[string]any{"message": "Missing Authorization bearer token"},
		})
		return
	}
	payload, err := readJSONBody(w, r, h.Config.MaxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if err == errBodyTooLarge {
			status = http.StatusRequestEntityTooLarge
		}
		log.Warn("rejected request body", "error", err.Error(), "status", status)
		writeJSON(w, status, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}

	attrs := chatPayloadSummary(payload)
	log.Info("chat completion request", attrs...)
	if h.Config.Verbose {
		log.Debug("client request headers", "headers", redactAuthorizationHeader(r))
		summarizeJSONBody("client request body", mustMarshal(payload))
	}

	cfg := h.configForRequest(payload)
	prepared := transform.PrepareUpstreamRequest(payload, cfg, h.Store, auth)
	log.Info("upstream request prepared", preparedSummary(prepared)...)
	if h.Config.MissingReasoningStrategy == "reject" && prepared.MissingReasoningMessages > 0 {
		log.Warn(
			"rejected missing reasoning",
			"missing_reasoning_messages", prepared.MissingReasoningMessages,
		)
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": map[string]any{
				"message": fmt.Sprintf(
					"dsproxy strict mode: missing reasoning_content for %d assistant message(s)",
					prepared.MissingReasoningMessages,
				),
				"type":                       "missing_reasoning_content",
				"code":                       "missing_reasoning_content",
				"missing_reasoning_messages": prepared.MissingReasoningMessages,
			},
		})
		return
	}

	upstreamBody, err := json.Marshal(prepared.Payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}

	upstreamURL := h.Config.UpstreamBaseURL + "/chat/completions"
	streaming, _ := prepared.Payload["stream"].(bool)
	log.Info("forwarding upstream", "url", upstreamURL, "stream", streaming, "body_bytes", len(upstreamBody))
	if h.Config.Verbose {
		summarizeJSONBody("upstream request body", upstreamBody)
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(upstreamBody))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("Accept-Encoding", "identity")
	if lang := r.Header.Get("Accept-Language"); lang != "" {
		req.Header.Set("Accept-Language", lang)
	}

	upstreamStarted := time.Now()
	resp, err := h.Client.Do(req)
	if err != nil {
		log.Error("upstream request failed", "error", err, "duration_ms", time.Since(upstreamStarted).Milliseconds())
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{"message": fmt.Sprintf("Upstream request failed: %v", err)},
		})
		return
	}
	defer resp.Body.Close()
	log.Info(
		"upstream response",
		"status", resp.StatusCode,
		"stream", streaming,
		"duration_ms", time.Since(upstreamStarted).Milliseconds(),
	)

	// Write trace if enabled (after we know the response status).
	if h.Config.TraceDir != "" {
		h.writeChatTrace(r, req, upstreamBody, resp.StatusCode)
	}

	if streaming {
		h.proxyStreaming(w, resp, prepared, started)
		return
	}
	h.proxyRegular(w, resp, prepared)
}

func (h *Handler) proxyRegular(w http.ResponseWriter, resp *http.Response, prepared transform.PreparedRequest) {
	body, err := readResponseBody(resp)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	contexts := prepared.RecordResponseContexts
	rewritten, err := transform.RewriteResponseBody(
		body,
		prepared.OriginalModel,
		h.Store,
		prepared.RecordResponseMessages,
		prepared.CacheNamespace,
		prepared.RecoveryNotice,
		prepared.RecordResponseScope,
		prepared.RecordResponseMessages,
		contexts,
		h.Config.DisplayReasoning,
		h.Config.CollapsibleReasoning,
	)
	if err != nil {
		rewritten = body
	}
	if h.Config.Verbose {
		summarizeJSONBody("client response body", rewritten)
	}
	log.Info("chat completion response", "status", resp.StatusCode, "body_bytes", len(rewritten))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(rewritten)
}

func (h *Handler) proxyStreaming(w http.ResponseWriter, resp *http.Response, prepared transform.PreparedRequest, started time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "close")
	w.WriteHeader(resp.StatusCode)

	flusher, ok := w.(http.Flusher)
	accumulator := stream.NewAccumulator()
	var display *stream.DisplayAdapter
	if h.Config.DisplayReasoning {
		display = stream.NewDisplayAdapter(h.Config.CollapsibleReasoning)
	}
	contexts := prepared.RecordResponseContexts
	recoveryNotice := prepared.RecoveryNotice
	finalized := false

	reader := bufioReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			rewritten, done, notice, _ := rewriteSSELine(
				line,
				prepared.OriginalModel,
				accumulator,
				h.Store,
				prepared.CacheNamespace,
				contexts,
				display,
				recoveryNotice,
			)
			recoveryNotice = notice
			if done {
				finalized = true
			}
			if _, writeErr := w.Write(rewritten); writeErr != nil {
				break
			}
			if ok {
				flusher.Flush()
			}
			if done {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if !finalized {
		for _, ctx := range contexts {
			_ = accumulator.StoreReasoning(h.Store, ctx.Scope, prepared.CacheNamespace, ctx.Messages)
		}
		log.Warn("streaming ended before done", "finalized", finalized)
	}
	log.Info(
		"chat completion stream finished",
		"upstream_status", resp.StatusCode,
		"finalized", finalized,
		"duration_ms", time.Since(started).Milliseconds(),
	)
}

// writeChatTrace builds a TraceEntry from the client and upstream requests and
// writes it to the configured trace directory.
func (h *Handler) writeChatTrace(clientReq *http.Request, upstreamReq *http.Request, upstreamBody []byte, upstreamStatus int) {
	clientHeaders := redactHeaderValues(headersToMap(clientReq.Header))
	upstreamHeaders := redactHeaderValues(headersToMap(upstreamReq.Header))

	entry := TraceEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Client: TraceRequestInfo{
			Method:  clientReq.Method,
			Path:    clientReq.URL.Path,
			Headers: clientHeaders,
			Body:    nil, // raw client body is logged via verbose flag
		},
		Upstream: TraceRequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamReq.URL.String(),
			Headers: upstreamHeaders,
			Body:    json.RawMessage(upstreamBody),
		},
		ResponseStatus: upstreamStatus,
	}

	writeTrace(h.Config.TraceDir, entry)
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

var errBodyTooLarge = fmt.Errorf("request body is too large")

func readJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int) (map[string]any, error) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			return nil, errBodyTooLarge
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return payload, nil
}

func cursorAuthorization(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || strings.TrimSpace(parts[1]) == "" {
		return ""
	}
	return "Bearer " + strings.TrimSpace(parts[1])
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	enc := strings.ToLower(resp.Header.Get("Content-Encoding"))
	switch enc {
	case "gzip":
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, nil
		}
		defer gr.Close()
		return io.ReadAll(gr)
	default:
		return body, nil
	}
}
