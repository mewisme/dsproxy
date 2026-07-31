package reasoning

import (
	"crypto/sha256"
	"dsproxy/internal/jsoncanon"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func NormalizeToolCall(toolCall map[string]any) map[string]any {
	if toolCall == nil {
		toolCall = map[string]any{}
	}
	function, _ := toolCall["function"].(map[string]any)
	if function == nil {
		function = map[string]any{}
	}
	arguments := function["arguments"]
	argStr := ""
	switch a := arguments.(type) {
	case string:
		argStr = a
	default:
		b, _ := json.Marshal(a)
		argStr = string(b)
	}
	normalized := map[string]any{
		"type": orDefault(toolCall["type"], "function"),
		"function": map[string]any{
			"name":      orDefault(function["name"], ""),
			"arguments": argStr,
		},
	}
	if id, ok := toolCall["id"]; ok && fmt.Sprint(id) != "" {
		normalized["id"] = fmt.Sprint(id)
	}
	return normalized
}

func ToolCallSignature(toolCall map[string]any) string {
	normalized := NormalizeToolCall(toolCall)
	delete(normalized, "id")
	return Sha256JSON(normalized)
}

func ToolCallIDs(message map[string]any) []string {
	var ids []string
	toolCalls, _ := message["tool_calls"].([]any)
	for _, tc := range toolCalls {
		m, ok := tc.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := m["id"]; ok && fmt.Sprint(id) != "" {
			ids = append(ids, fmt.Sprint(id))
		}
	}
	return ids
}

func ToolCallNames(message map[string]any) []string {
	var names []string
	toolCalls, _ := message["tool_calls"].([]any)
	for _, tc := range toolCalls {
		m, ok := tc.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := m["function"].(map[string]any)
		if fn != nil && fn["name"] != nil && fmt.Sprint(fn["name"]) != "" {
			names = append(names, fmt.Sprint(fn["name"]))
		}
	}
	return names
}

func MessageSignature(message map[string]any) string {
	var toolCalls []map[string]any
	if raw, ok := message["tool_calls"].([]any); ok {
		for _, tc := range raw {
			if m, ok := tc.(map[string]any); ok {
				toolCalls = append(toolCalls, NormalizeToolCall(m))
			}
		}
	}
	content := message["content"]
	if content == nil {
		content = ""
	}
	payload := map[string]any{
		"content":    content,
		"tool_calls": toolCalls,
	}
	return Sha256JSON(payload)
}

func CanonicalScopeMessage(message map[string]any) map[string]any {
	canonical := map[string]any{"role": message["role"]}
	for _, key := range []string{"content", "name", "tool_call_id", "prefix"} {
		if v, ok := message[key]; ok {
			canonical[key] = v
		}
	}
	if raw, ok := message["tool_calls"].([]any); ok && len(raw) > 0 {
		var toolCalls []map[string]any
		for _, tc := range raw {
			if m, ok := tc.(map[string]any); ok {
				toolCalls = append(toolCalls, NormalizeToolCall(m))
			}
		}
		canonical["tool_calls"] = toolCalls
	}
	return canonical
}

func ConversationScope(messages []map[string]any, namespace string) string {
	scopeMessages := make([]map[string]any, len(messages))
	for i, m := range messages {
		scopeMessages[i] = CanonicalScopeMessage(m)
	}
	var payload any = scopeMessages
	if namespace != "" {
		payload = map[string]any{
			"namespace": namespace,
			"messages":  scopeMessages,
		}
	}
	return Sha256JSON(payload)
}

func TurnContextSignature(priorMessages []map[string]any) string {
	lastUser := -1
	for i := len(priorMessages) - 1; i >= 0; i-- {
		if priorMessages[i]["role"] == "user" {
			lastUser = i
			break
		}
	}
	start := 0
	if lastUser != -1 {
		start = lastUser
		for start > 0 && priorMessages[start-1]["role"] == "user" {
			start--
		}
	}
	var context []map[string]any
	for _, m := range priorMessages[start:] {
		if m["role"] == "system" {
			continue
		}
		context = append(context, CanonicalScopeMessage(m))
	}
	return Sha256JSON(context)
}

func ScopedReasoningKeys(message map[string]any, scope string) []string {
	keys := []string{fmt.Sprintf("scope:%s:signature:%s", scope, MessageSignature(message))}
	for _, id := range ToolCallIDs(message) {
		keys = append(keys, fmt.Sprintf("scope:%s:tool_call:%s", scope, id))
	}
	if raw, ok := message["tool_calls"].([]any); ok {
		for _, tc := range raw {
			if m, ok := tc.(map[string]any); ok {
				keys = append(keys, fmt.Sprintf("scope:%s:tool_call_signature:%s", scope, ToolCallSignature(m)))
			}
		}
	}
	for _, name := range ToolCallNames(message) {
		keys = append(keys, fmt.Sprintf("scope:%s:tool_name:%s", scope, name))
	}
	return keys
}

func PortableReasoningKeys(message map[string]any, cacheNamespace string, priorMessages []map[string]any) []string {
	if cacheNamespace == "" {
		return nil
	}
	turnSig := TurnContextSignature(priorMessages)
	keys := []string{
		fmt.Sprintf("namespace:%s:turn:%s:signature:%s", cacheNamespace, turnSig, MessageSignature(message)),
	}
	for _, id := range ToolCallIDs(message) {
		keys = append(keys, fmt.Sprintf("namespace:%s:turn:%s:tool_call:%s", cacheNamespace, turnSig, id))
	}
	if raw, ok := message["tool_calls"].([]any); ok {
		for _, tc := range raw {
			if m, ok := tc.(map[string]any); ok {
				keys = append(keys, fmt.Sprintf("namespace:%s:turn:%s:tool_call_signature:%s", cacheNamespace, turnSig, ToolCallSignature(m)))
			}
		}
	}
	for _, name := range ToolCallNames(message) {
		keys = append(keys, fmt.Sprintf("namespace:%s:turn:%s:tool_name:%s", cacheNamespace, turnSig, name))
	}
	return keys
}

func Sha256JSON(payload any) string {
	b, err := jsoncanon.Marshal(payload)
	if err != nil {
		b, _ = json.Marshal(payload)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func orDefault(v any, def string) string {
	if v == nil || fmt.Sprint(v) == "" {
		return def
	}
	return fmt.Sprint(v)
}

func uniqueKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}
