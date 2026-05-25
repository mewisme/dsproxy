package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"dsproxy/internal/config"
	"dsproxy/internal/proxy"
	"dsproxy/internal/reasoning"
)

const (
	thinking11 = "Thinking 1.1 - need to look up the date."
	thinking12 = "Thinking 1.2 - I have the date, now I need the weather."
	thinking13 = "Thinking 1.3 - tool results suffice for the answer."
	thinking21 = "Thinking 2.1 - a brand new user turn."
	answer1    = "Answer 1: Tomorrow is sunny on 2026-04-24."
	answer2    = "Answer 2: Acknowledged follow-up."
	callID1    = "call_get_date"
	callID2    = "call_get_weather"
)

var tools = []any{
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":       "get_date",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "get_weather",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"date": map[string]any{"type": "string"}},
				"required":   []any{"date"},
			},
		},
	},
}

var upstreamRequests []map[string]any

func TestCanonicalFourTurnToolCallLoop(t *testing.T) {
	upstreamRequests = nil
	upstream := httptest.NewServer(http.HandlerFunc(strictFakeDeepSeek))
	defer upstream.Close()

	store, err := reasoning.Open(":memory:", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := config.ProxyConfig{
		Host:                     "0.0.0.0",
		Port:                     0,
		UpstreamBaseURL:          upstream.URL,
		UpstreamModel:            "deepseek-v4-pro",
		Thinking:                 "enabled",
		ReasoningEffort:          "max",
		MissingReasoningStrategy: "recover",
		DisplayReasoning:         false,
		MaxRequestBodyBytes:      config.DefaultMaxRequestBodyBytes,
		RequestTimeout:           30,
	}
	proxySrv := httptest.NewServer(proxy.NewServer(cfg, store).HTTP.Handler)
	defer proxySrv.Close()

	// Turn 1.1
	status, resp := post(t, proxySrv.URL+"/v1/chat/completions", map[string]any{
		"model": "deepseek-v4-pro",
		"messages": []any{
			map[string]any{"role": "user", "content": "What's the weather tomorrow?"},
		},
		"tools": tools,
	})
	if status != 200 {
		t.Fatalf("turn 1.1 status=%d body=%v", status, resp)
	}
	first := messageFrom(resp)
	if first["reasoning_content"] != thinking11 {
		t.Fatalf("reasoning=%v", first["reasoning_content"])
	}

	// Turn 1.2
	status, resp = post(t, proxySrv.URL+"/v1/chat/completions", map[string]any{
		"model": "deepseek-v4-pro",
		"messages": []any{
			map[string]any{"role": "user", "content": "What's the weather tomorrow?"},
			dropReasoning(first),
			map[string]any{"role": "tool", "tool_call_id": callID1, "content": "2026-04-24"},
		},
		"tools": tools,
	})
	if status != 200 {
		t.Fatalf("turn 1.2 status=%d", status)
	}
	second := messageFrom(resp)
	upstream12 := upstreamRequests[1]["messages"].([]any)
	msg12 := upstream12[1].(map[string]any)
	if msg12["reasoning_content"] != thinking11 {
		t.Fatalf("upstream 1.2 reasoning=%v", msg12["reasoning_content"])
	}

	// Turn 1.3
	status, resp = post(t, proxySrv.URL+"/v1/chat/completions", map[string]any{
		"model": "deepseek-v4-pro",
		"messages": []any{
			map[string]any{"role": "user", "content": "What's the weather tomorrow?"},
			dropReasoning(first),
			map[string]any{"role": "tool", "tool_call_id": callID1, "content": "2026-04-24"},
			dropReasoning(second),
			map[string]any{"role": "tool", "tool_call_id": callID2, "content": "sunny"},
		},
		"tools": tools,
	})
	if status != 200 {
		t.Fatalf("turn 1.3 status=%d", status)
	}
	third := messageFrom(resp)
	upstream13 := upstreamRequests[2]["messages"].([]any)
	if upstream13[1].(map[string]any)["reasoning_content"] != thinking11 {
		t.Fatal("upstream 1.3 missing thinking 1.1")
	}
	if upstream13[3].(map[string]any)["reasoning_content"] != thinking12 {
		t.Fatal("upstream 1.3 missing thinking 1.2")
	}
	_ = third

	// Turn 2.1
	status, resp = post(t, proxySrv.URL+"/v1/chat/completions", map[string]any{
		"model": "deepseek-v4-pro",
		"messages": []any{
			map[string]any{"role": "user", "content": "What's the weather tomorrow?"},
			dropReasoning(first),
			map[string]any{"role": "tool", "tool_call_id": callID1, "content": "2026-04-24"},
			dropReasoning(second),
			map[string]any{"role": "tool", "tool_call_id": callID2, "content": "sunny"},
			dropReasoning(third),
			map[string]any{"role": "user", "content": "Thanks. What about Saturday?"},
		},
		"tools": tools,
	})
	if status != 200 {
		t.Fatalf("turn 2.1 status=%d", status)
	}
	upstream21 := upstreamRequests[3]["messages"].([]any)
	if upstream21[1].(map[string]any)["reasoning_content"] != thinking11 {
		t.Fatal("upstream 2.1 missing thinking 1.1")
	}
	if upstream21[3].(map[string]any)["reasoning_content"] != thinking12 {
		t.Fatal("upstream 2.1 missing thinking 1.2")
	}
	if upstream21[5].(map[string]any)["reasoning_content"] != thinking13 {
		t.Fatal("upstream 2.1 missing thinking 1.3")
	}
	content := messageFrom(resp)["content"].(string)
	if !contains(content, answer2) {
		t.Fatalf("content=%q", content)
	}
}

func strictFakeDeepSeek(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	upstreamRequests = append(upstreamRequests, payload)
	messages, _ := payload["messages"].([]any)

	for i, raw := range messages {
		msg, _ := raw.(map[string]any)
		if msg["role"] != "assistant" {
			continue
		}
		if isToolTurnAssistant(messages, i) && msg["reasoning_content"] == nil {
			writeJSON(w, 400, map[string]any{
				"error": map[string]any{
					"message": "The reasoning_content in the thinking mode must be passed back to the API.",
					"type":    "invalid_request_error",
				},
			})
			return
		}
	}

	lastUser := lastIndex(messages, "user")
	lastTool := lastIndex(messages, "tool")
	lastAssistant := lastIndex(messages, "assistant")

	switch {
	case lastUser == 0 && lastAssistant == -1 && lastTool == -1:
		writeJSON(w, 200, completion("chatcmpl-1-1", "tool_calls", "", thinking11, []any{
			map[string]any{
				"id": callID1, "type": "function",
				"function": map[string]any{"name": "get_date", "arguments": "{}"},
			},
		}))
	case lastUser > 0 && lastUser > lastTool:
		writeJSON(w, 200, completion("chatcmpl-2-1", "stop", answer2, thinking21, nil))
	case lastTool != -1 && messages[lastTool].(map[string]any)["tool_call_id"] == callID1 && lastAssistant < lastTool:
		writeJSON(w, 200, completion("chatcmpl-1-2", "tool_calls", "", thinking12, []any{
			map[string]any{
				"id": callID2, "type": "function",
				"function": map[string]any{"name": "get_weather", "arguments": `{"date":"2026-04-24"}`},
			},
		}))
	case lastTool != -1 && messages[lastTool].(map[string]any)["tool_call_id"] == callID2:
		writeJSON(w, 200, completion("chatcmpl-1-3", "stop", answer1, thinking13, nil))
	default:
		writeJSON(w, 400, map[string]any{"error": map[string]any{"message": "unexpected shape"}})
	}
}

func completion(id, finishReason, content, reasoning string, toolCalls []any) map[string]any {
	msg := map[string]any{"role": "assistant", "content": content}
	if reasoning != "" {
		msg["reasoning_content"] = reasoning
	}
	if toolCalls != nil {
		msg["tool_calls"] = toolCalls
	}
	return map[string]any{
		"id": id, "object": "chat.completion", "created": 1, "model": "deepseek-v4-pro",
		"choices": []any{map[string]any{"index": 0, "finish_reason": finishReason, "message": msg}},
	}
}

func isToolTurnAssistant(messages []any, index int) bool {
	msg := messages[index].(map[string]any)
	if tc, ok := msg["tool_calls"].([]any); ok && len(tc) > 0 {
		return true
	}
	for i := index - 1; i >= 0; i-- {
		role := messages[i].(map[string]any)["role"]
		if role == "tool" {
			return true
		}
		if role == "user" || role == "system" {
			return false
		}
	}
	return false
}

func lastIndex(messages []any, role string) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].(map[string]any)["role"] == role {
			return i
		}
	}
	return -1
}

func post(t *testing.T, url string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

func messageFrom(resp map[string]any) map[string]any {
	choices := resp["choices"].([]any)
	choice := choices[0].(map[string]any)
	return choice["message"].(map[string]any)
}

func dropReasoning(msg map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range msg {
		if k != "reasoning_content" {
			out[k] = v
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
