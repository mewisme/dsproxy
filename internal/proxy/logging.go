package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dsproxy/internal/log"

	"dsproxy/internal/transform"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (h *Handler) logIncoming(r *http.Request) {
	attrs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"remote", r.RemoteAddr,
		"content_length", r.ContentLength,
	}
	if ua := r.UserAgent(); ua != "" {
		attrs = append(attrs, "user_agent", ua)
	}
	if h.hasAuthorization(r) {
		attrs = append(attrs, "authorization", "bearer")
	}
	log.Info("incoming request", attrs...)
}

func (h *Handler) logCompleted(r *http.Request, rec *responseRecorder, started time.Time, extra ...any) {
	attrs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"status", rec.status,
		"response_bytes", rec.bytes,
		"duration_ms", time.Since(started).Milliseconds(),
		"remote", r.RemoteAddr,
	}
	attrs = append(attrs, extra...)
	log.Info("request completed", attrs...)
}

func (h *Handler) hasAuthorization(r *http.Request) bool {
	return cursorAuthorization(r) != ""
}

func chatPayloadSummary(payload map[string]any) []any {
	attrs := []any{
		"model", payload["model"],
		"stream", payload["stream"],
	}
	if msgs, ok := payload["messages"].([]any); ok {
		attrs = append(attrs, "messages", len(msgs))
	}
	if tools, ok := payload["tools"].([]any); ok {
		attrs = append(attrs, "tools", len(tools))
	}
	if tc := payload["tool_choice"]; tc != nil {
		attrs = append(attrs, "tool_choice", tc)
	}
	return attrs
}

func preparedSummary(prepared transform.PreparedRequest) []any {
	return []any{
		"original_model", prepared.OriginalModel,
		"upstream_model", prepared.UpstreamModel,
		"patched_reasoning", prepared.PatchedReasoningMessages,
		"missing_reasoning", prepared.MissingReasoningMessages,
		"recovered_reasoning", prepared.RecoveredReasoningMessages,
	}
}

func summarizeJSONBody(label string, body []byte) {
	if len(body) == 0 {
		log.Debug(label, "body", "(empty)")
		return
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Debug(label, "body_bytes", len(body), "parse_error", err.Error())
		return
	}
	log.Debug(label, "body", payload)
}

func redactAuthorizationHeader(r *http.Request) http.Header {
	clone := r.Header.Clone()
	if auth := clone.Get("Authorization"); auth != "" {
		clone.Set("Authorization", redactBearer(auth))
	}
	return clone
}

func redactBearer(auth string) string {
	parts := strings.SplitN(strings.TrimSpace(auth), " ", 2)
	if len(parts) != 2 {
		return "[redacted]"
	}
	token := parts[1]
	if len(token) <= 8 {
		return parts[0] + " [redacted]"
	}
	return parts[0] + " " + token[:4] + "…" + token[len(token)-4:]
}
