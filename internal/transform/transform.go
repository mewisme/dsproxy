package transform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"dsproxy/internal/config"
	"dsproxy/internal/reasoning"
)

const (
	RecoveryNoticeText    = "[dsproxy] Refreshed reasoning_content history."
	RecoveryNoticeContent = RecoveryNoticeText + "\n\n"
	recoverySystemContent = "dsproxy recovered this request because older DeepSeek " +
		"thinking-mode tool-call reasoning_content was unavailable. Older " +
		"unrecoverable tool-call history was omitted; continue using only the " +
		"remaining recovered context."
)

var (
	supportedRequestFields = map[string]struct{}{
		"model": {}, "messages": {}, "stream": {}, "stream_options": {},
		"max_tokens": {}, "response_format": {}, "stop": {}, "tools": {},
		"tool_choice": {}, "thinking": {}, "reasoning_effort": {},
		"temperature": {}, "top_p": {}, "presence_penalty": {}, "frequency_penalty": {},
		"logprobs": {}, "top_logprobs": {}, "user": {}, "seed": {}, "n": {}, "logit_bias": {},
	}
	messageFields = map[string]struct{}{
		"role": {}, "content": {}, "name": {}, "tool_call_id": {},
		"tool_calls": {}, "reasoning_content": {}, "prefix": {},
	}
	roleMessageFields = map[string]map[string]struct{}{
		"system":    {"role": {}, "content": {}, "name": {}},
		"user":      {"role": {}, "content": {}, "name": {}},
		"assistant": {"role": {}, "content": {}, "name": {}, "tool_calls": {}, "reasoning_content": {}, "prefix": {}},
		"tool":      {"role": {}, "content": {}, "tool_call_id": {}},
	}
	effortAliases = map[string]string{
		"low": "high", "medium": "high", "high": "high", "max": "max", "xhigh": "max",
	}
	cursorThinkingBlockRE = regexp.MustCompile(`(?is)(?:<(?:think|thinking)\b[^>]*>[\s\S]*?(?:</(?:think|thinking)>|\z)|<details\b[^>]*>\s*<summary\b[^>]*>\s*Thinking\s*</summary>[\s\S]*?(?:</details>|\z))\s*`)
)

type PreparedRequest struct {
	Payload                    map[string]any
	OriginalModel              string
	UpstreamModel              string
	CacheNamespace             string
	PatchedReasoningMessages   int
	MissingReasoningMessages   int
	RecoveredReasoningMessages int
	RecoveryDroppedMessages    int
	RecoveryNotice             string
	RecordResponseScope        string
	RecordResponseMessages     []map[string]any
	RecordResponseContexts     []RecordingContext
}

type RecordingContext struct {
	Scope    string
	Messages []map[string]any
}

func NormalizeReasoningEffort(value any) string {
	s, ok := value.(string)
	if !ok {
		return "high"
	}
	if v, ok := effortAliases[strings.ToLower(strings.TrimSpace(s))]; ok {
		return v
	}
	return "high"
}

func ExtractTextContent(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	if list, ok := content.([]any); ok {
		var parts []string
		for _, item := range list {
			switch v := item.(type) {
			case string:
				parts = append(parts, v)
			case map[string]any:
				itemType, _ := v["type"].(string)
				text, _ := v["text"].(string)
				if text == "" {
					text, _ = v["content"].(string)
				}
				if (itemType == "text" || itemType == "input_text") && text != "" {
					parts = append(parts, text)
				} else if text != "" {
					parts = append(parts, text)
				} else if itemType != "" {
					parts = append(parts, fmt.Sprintf("[%s omitted by DeepSeek text proxy]", itemType))
				}
			default:
				parts = append(parts, fmt.Sprint(v))
			}
		}
		return strings.Join(parts, "\n")
	}
	b, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprint(content)
	}
	return string(b)
}

func StripCursorThinkingBlocks(content string) string {
	return strings.TrimLeft(cursorThinkingBlockRE.ReplaceAllString(content, ""), "\r\n")
}

func NormalizeToolCall(toolCall any) map[string]any {
	m, _ := toolCall.(map[string]any)
	return reasoning.NormalizeToolCall(m)
}

func NormalizeTool(tool any) map[string]any {
	m, ok := tool.(map[string]any)
	if !ok {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "", "description": "", "parameters": map[string]any{},
			},
		}
	}
	out := copyMap(m)
	if out["type"] == nil {
		out["type"] = "function"
	}
	return out
}

func LegacyFunctionToTool(function any) map[string]any {
	fn, _ := function.(map[string]any)
	if fn == nil {
		fn = map[string]any{}
	}
	return map[string]any{"type": "function", "function": fn}
}

func ConvertFunctionCall(functionCall any) any {
	if s, ok := functionCall.(string); ok {
		if s == "auto" || s == "none" || s == "required" {
			return s
		}
		return nil
	}
	if m, ok := functionCall.(map[string]any); ok && m["name"] != nil {
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": fmt.Sprint(m["name"])},
		}
	}
	return nil
}

func NormalizeToolChoice(toolChoice any) any {
	if s, ok := toolChoice.(string); ok {
		if s == "auto" || s == "none" || s == "required" {
			return s
		}
		return nil
	}
	if m, ok := toolChoice.(map[string]any); ok {
		if m["type"] == "function" {
			if fn, ok := m["function"].(map[string]any); ok && fn["name"] != nil {
				return map[string]any{
					"type":     "function",
					"function": map[string]any{"name": fmt.Sprint(fn["name"])},
				}
			}
		}
		return m
	}
	return toolChoice
}

func AssistantNeedsReasoning(message map[string]any, prior []map[string]any) bool {
	if _, ok := message["tool_calls"]; ok {
		if raw, ok := message["tool_calls"].([]any); ok && len(raw) > 0 {
			return true
		}
	}
	for i := len(prior) - 1; i >= 0; i-- {
		role, _ := prior[i]["role"].(string)
		if role == "tool" {
			return true
		}
		if role == "user" || role == "system" {
			return false
		}
	}
	return false
}

func HasRecoveryNotice(message map[string]any) bool {
	content, _ := message["content"].(string)
	return message["role"] == "assistant" && strings.HasPrefix(content, RecoveryNoticeText)
}

func StripRecoveryNoticeForUpstream(messages []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message["role"] != "assistant" {
			out = append(out, message)
			continue
		}
		content, _ := message["content"].(string)
		if !strings.HasPrefix(content, RecoveryNoticeText) {
			out = append(out, message)
			continue
		}
		cleaned := copyMap(message)
		cleaned["content"] = strings.TrimLeft(content[len(RecoveryNoticeText):], "\r\n")
		out = append(out, cleaned)
	}
	return out
}

func LeadingSystemMessages(messages []map[string]any) []map[string]any {
	var leading []map[string]any
	for _, m := range messages {
		if m["role"] == "system" {
			leading = append(leading, m)
			continue
		}
		break
	}
	return leading
}

func ActiveMessagesFromRecoveryBoundary(messages []map[string]any) ([]map[string]any, int, bool) {
	recoveryIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if HasRecoveryNotice(messages[i]) {
			recoveryIdx = i
			break
		}
	}
	if recoveryIdx == -1 {
		return nil, 0, false
	}
	contextUser := -1
	for i := recoveryIdx - 1; i >= 0; i-- {
		if messages[i]["role"] == "user" {
			contextUser = i
			break
		}
	}
	leading := LeadingSystemMessages(messages)
	var tail []map[string]any
	if contextUser != -1 {
		tail = append(tail, messages[contextUser])
	}
	tail = append(tail, messages[recoveryIdx:]...)
	active := append(append([]map[string]any{}, leading...),
		map[string]any{"role": "system", "content": recoverySystemContent})
	active = append(active, tail...)
	kept := 0
	if contextUser != -1 {
		kept = 1
	}
	retired := recoveryIdx - len(leading) - kept
	if retired < 0 {
		retired = 0
	}
	return active, retired, true
}

func RecoverMessagesFromMissingReasoning(messages []map[string]any, missingIndexes []int) ([]map[string]any, int, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		if !HasRecoveryNotice(messages[i]) {
			continue
		}
		hasEarlierMissing := false
		for _, mi := range missingIndexes {
			if mi < i {
				hasEarlierMissing = true
				break
			}
		}
		if !hasEarlierMissing {
			continue
		}
		contextUser := -1
		for j := i - 1; j >= 0; j-- {
			if messages[j]["role"] == "user" {
				contextUser = j
				break
			}
		}
		leading := LeadingSystemMessages(messages)
		var tail []map[string]any
		if contextUser != -1 {
			tail = append(tail, messages[contextUser])
		}
		tail = append(tail, messages[i:]...)
		recovered := append(append([]map[string]any{}, leading...),
			map[string]any{"role": "system", "content": recoverySystemContent})
		recovered = append(recovered, tail...)
		kept := 0
		if contextUser != -1 {
			kept = 1
		}
		omitted := i - len(leading) - kept
		return recovered, omitted, ""
	}

	lastUser := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i]["role"] == "user" {
			lastUser = i
			break
		}
	}
	if lastUser == -1 {
		return messages, 0, ""
	}
	recovered := LeadingSystemMessages(messages)
	omitted := len(messages) - len(recovered) - 1
	recovered = append(recovered, map[string]any{"role": "system", "content": recoverySystemContent})
	recovered = append(recovered, messages[lastUser])
	return recovered, omitted, RecoveryNoticeContent
}

func NormalizeMessage(
	message any,
	store *reasoning.Store,
	prior []map[string]any,
	cacheNamespace string,
	repairReasoning, keepReasoning bool,
) (map[string]any, bool, bool) {
	m, ok := message.(map[string]any)
	if !ok {
		m = map[string]any{"role": "user", "content": fmt.Sprint(message)}
	}
	normalized := map[string]any{}
	for k, v := range m {
		if _, allowed := messageFields[k]; allowed {
			normalized[k] = v
		}
	}
	role, _ := normalized["role"].(string)
	if role == "" {
		role = "user"
	}
	if role == "function" {
		role = "tool"
	}
	normalized["role"] = role

	if _, hasContent := normalized["content"]; hasContent {
		normalized["content"] = ExtractTextContent(normalized["content"])
	} else if role == "assistant" || role == "tool" || role == "system" || role == "user" {
		normalized["content"] = ""
	}
	if role == "assistant" {
		if c, ok := normalized["content"].(string); ok {
			normalized["content"] = StripCursorThinkingBlocks(c)
		}
	}
	if raw, ok := normalized["tool_calls"].([]any); ok && len(raw) > 0 {
		var tcs []any
		for _, tc := range raw {
			tcs = append(tcs, NormalizeToolCall(tc))
		}
		normalized["tool_calls"] = tcs
	}

	patched, missing := false, false
	if role == "assistant" {
		if !keepReasoning {
			delete(normalized, "reasoning_content")
		} else if repairReasoning {
			_, hasReasoning := normalized["reasoning_content"].(string)
			if !hasReasoning {
				delete(normalized, "reasoning_content")
				if AssistantNeedsReasoning(normalized, prior) {
					lookupScope := reasoning.ConversationScope(prior, cacheNamespace)
					if store != nil {
						if restored, ok := store.LookupForMessage(normalized, lookupScope, cacheNamespace, prior); ok {
							normalized["reasoning_content"] = restored
							patched = true
							store.BackfillPortableAliases(normalized, restored, cacheNamespace, prior)
						} else {
							missing = true
						}
					} else {
						missing = true
					}
				}
			}
		}
	}

	allowed := roleMessageFields[role]
	if allowed == nil {
		allowed = messageFields
	}
	filtered := map[string]any{}
	for k, v := range normalized {
		if _, ok := allowed[k]; ok {
			filtered[k] = v
		}
	}
	return filtered, patched, missing
}

func NormalizeMessages(
	messages any,
	store *reasoning.Store,
	cacheNamespace string,
	repairReasoning, keepReasoning bool,
) ([]map[string]any, int, []int) {
	list, ok := messages.([]any)
	if !ok {
		if typed, ok := messages.([]map[string]any); ok {
			list = make([]any, len(typed))
			for i, m := range typed {
				list[i] = m
			}
		} else {
			return nil, 0, nil
		}
	}
	var normalized []map[string]any
	patchedCount := 0
	var missing []int
	for _, item := range list {
		m, patched, miss := NormalizeMessage(item, store, normalized, cacheNamespace, repairReasoning, keepReasoning)
		normalized = append(normalized, m)
		if patched {
			patchedCount++
		}
		if miss {
			missing = append(missing, len(normalized)-1)
		}
	}
	return normalized, patchedCount, missing
}

func UpstreamModelFor(original string, cfg config.ProxyConfig) string {
	original = strings.TrimSpace(original)
	if original == "" {
		return cfg.UpstreamModel
	}
	return original
}

func normalizedSystemMessages(messages []map[string]any) []map[string]any {
	var result []map[string]any
	for _, m := range messages {
		if m["role"] != "system" {
			continue
		}
		cm := map[string]any{"role": m["role"], "content": m["content"]}
		if name, ok := m["name"]; ok {
			cm["name"] = name
		}
		result = append(result, cm)
	}
	return result
}

func normalizedToolsForContext(tools any) []any {
	if tools == nil {
		return []any{}
	}
	t, _ := tools.([]any)
	if t == nil {
		return []any{}
	}
	return t
}

func effectiveToolChoice(tools []any, explicit any) any {
	if explicit != nil {
		return explicit
	}
	if len(tools) > 0 {
		return "auto"
	}
	return "none"
}

func RuntimeContextHash(systemMessages []map[string]any, tools any, toolChoice any) string {
	var canonical []map[string]any
	for _, m := range systemMessages {
		if m["role"] != "system" {
			continue
		}
		cm := map[string]any{"role": m["role"], "content": m["content"]}
		if name, ok := m["name"]; ok {
			cm["name"] = name
		}
		canonical = append(canonical, cm)
	}
	t := normalizedToolsForContext(tools)
	payload := map[string]any{
		"system_messages": canonical,
		"tools":           t,
		"tool_choice":     effectiveToolChoice(t, toolChoice),
	}
	return reasoning.Sha256JSON(payload)
}

func ReasoningCacheNamespace(cfg config.ProxyConfig, upstreamModel string, thinking, reasoningEffort any, authorization string, runtimeContextHash string) string {
	authHash := ""
	if authorization != "" {
		sum := sha256.Sum256([]byte(authorization))
		authHash = hex.EncodeToString(sum[:])
	}
	payload := map[string]any{
		"v":                    "v2",
		"base_url":             cfg.UpstreamBaseURL,
		"model":                upstreamModel,
		"thinking":             thinking,
		"reasoning_effort":     reasoningEffort,
		"authorization_hash":   authHash,
		"runtime_context_hash": runtimeContextHash,
	}
	return reasoning.Sha256JSON(payload)
}

func ResponseRecordingContexts(items ...*RecordingContext) []RecordingContext {
	var contexts []RecordingContext
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil {
			continue
		}
		if _, ok := seen[item.Scope]; ok {
			continue
		}
		seen[item.Scope] = struct{}{}
		contexts = append(contexts, *item)
	}
	return contexts
}

func PrepareUpstreamRequest(
	payload map[string]any,
	cfg config.ProxyConfig,
	store *reasoning.Store,
	authorization string,
) PreparedRequest {
	originalModel := cfg.UpstreamModel
	if m, ok := payload["model"].(string); ok && m != "" {
		originalModel = m
	}
	upstreamModel := UpstreamModelFor(originalModel, cfg)

	prepared := map[string]any{}
	for k, v := range payload {
		if _, ok := supportedRequestFields[k]; ok {
			prepared[k] = v
		}
	}
	if prepared["max_tokens"] == nil {
		if v, ok := payload["max_completion_tokens"]; ok {
			prepared["max_tokens"] = v
		}
	}
	prepared["model"] = upstreamModel

	if stream, _ := prepared["stream"].(bool); stream {
		opts, _ := prepared["stream_options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
		} else {
			opts = copyMap(opts)
		}
		opts["include_usage"] = true
		prepared["stream_options"] = opts
	}

	if tools, ok := prepared["tools"].([]any); ok {
		var normalized []any
		for _, t := range tools {
			normalized = append(normalized, NormalizeTool(t))
		}
		prepared["tools"] = normalized
	} else if functions, ok := payload["functions"].([]any); ok {
		var tools []any
		for _, f := range functions {
			tools = append(tools, LegacyFunctionToTool(f))
		}
		prepared["tools"] = tools
	}

	if _, ok := prepared["tool_choice"]; ok {
		tc := NormalizeToolChoice(prepared["tool_choice"])
		if tc == nil {
			delete(prepared, "tool_choice")
		} else {
			prepared["tool_choice"] = tc
		}
	} else if fc, ok := payload["function_call"]; ok {
		if tc := ConvertFunctionCall(fc); tc != nil {
			prepared["tool_choice"] = tc
		}
	}

	prepared["thinking"] = map[string]any{"type": cfg.Thinking}
	thinkingEnabled := cfg.Thinking == "enabled"
	thinkingDisabled := cfg.Thinking == "disabled"
	if thinkingEnabled {
		prepared["reasoning_effort"] = NormalizeReasoningEffort(cfg.ReasoningEffort)
	}

	preRepair, _, _ := NormalizeMessages(payload["messages"], nil, "", false, !thinkingDisabled)
	systemMessages := normalizedSystemMessages(preRepair)
	tools := normalizedToolsForContext(prepared["tools"])
	toolChoice := effectiveToolChoice(tools, prepared["tool_choice"])
	contextHash := RuntimeContextHash(systemMessages, tools, toolChoice)
	cacheNamespace := ReasoningCacheNamespace(cfg, upstreamModel, prepared["thinking"], prepared["reasoning_effort"], authorization, contextHash)
	recordMessages := preRepair
	recordScope := reasoning.ConversationScope(recordMessages, cacheNamespace)
	messagesForRepair := preRepair

	if thinkingEnabled && cfg.MissingReasoningStrategy == "recover" {
		if active, _, ok := ActiveMessagesFromRecoveryBoundary(preRepair); ok {
			messagesForRepair = active
		}
	}

	messages, patchedCount, missingIndexes := NormalizeMessages(messagesForRepair, store, cacheNamespace, thinkingEnabled, !thinkingDisabled)
	recoveredCount := 0
	recoveryDropped := 0
	recoveryNotice := ""
	for len(missingIndexes) > 0 && cfg.MissingReasoningStrategy == "recover" {
		recovered, dropped, notice := RecoverMessagesFromMissingReasoning(messages, missingIndexes)
		recoveredCount += len(missingIndexes)
		recoveryDropped += dropped
		if notice != "" {
			recoveryNotice = notice
		}
		if dropped == 0 {
			break
		}
		messages, patchedCount, missingIndexes = NormalizeMessages(recovered, store, cacheNamespace, thinkingEnabled, !thinkingDisabled)
	}

	activeScope := reasoning.ConversationScope(messages, cacheNamespace)
	contexts := ResponseRecordingContexts(
		&RecordingContext{Scope: recordScope, Messages: recordMessages},
		&RecordingContext{Scope: activeScope, Messages: messages},
	)
	prepared["messages"] = StripRecoveryNoticeForUpstream(messages)

	return PreparedRequest{
		Payload:                    prepared,
		OriginalModel:              originalModel,
		UpstreamModel:              upstreamModel,
		CacheNamespace:             cacheNamespace,
		PatchedReasoningMessages:   patchedCount,
		MissingReasoningMessages:   len(missingIndexes),
		RecoveredReasoningMessages: recoveredCount,
		RecoveryDroppedMessages:    recoveryDropped,
		RecoveryNotice:             recoveryNotice,
		RecordResponseScope:        recordScope,
		RecordResponseMessages:     recordMessages,
		RecordResponseContexts:     contexts,
	}
}

func RecordResponseReasoning(
	response map[string]any,
	store *reasoning.Store,
	requestMessages []map[string]any,
	cacheNamespace, scope string,
	priorMessages []map[string]any,
	contexts []RecordingContext,
) int {
	if store == nil {
		return 0
	}
	if len(contexts) == 0 {
		if scope == "" {
			scope = reasoning.ConversationScope(requestMessages, cacheNamespace)
		}
		if priorMessages == nil {
			priorMessages = requestMessages
		}
		contexts = []RecordingContext{{Scope: scope, Messages: priorMessages}}
	}
	stored := 0
	choices, _ := response["choices"].([]any)
	for _, ch := range choices {
		choice, _ := ch.(map[string]any)
		if choice == nil {
			continue
		}
		message, _ := choice["message"].(map[string]any)
		if message == nil {
			continue
		}
		for _, ctx := range contexts {
			stored += store.StoreAssistantMessage(message, ctx.Scope, cacheNamespace, ctx.Messages)
		}
	}
	return stored
}

func RewriteResponseBody(
	body []byte,
	originalModel string,
	store *reasoning.Store,
	requestMessages []map[string]any,
	cacheNamespace, contentPrefix string,
	scope string,
	priorMessages []map[string]any,
	contexts []RecordingContext,
	displayReasoning, collapsible bool,
) ([]byte, error) {
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return body, err
	}
	if contentPrefix != "" {
		PrefixResponseContent(response, contentPrefix)
	}
	RecordResponseReasoning(response, store, requestMessages, cacheNamespace, scope, priorMessages, contexts)
	if displayReasoning {
		FoldReasoningIntoContent(response, collapsible)
	}
	if _, ok := response["model"]; ok {
		response["model"] = originalModel
	}
	return json.Marshal(response)
}

func PrefixResponseContent(response map[string]any, prefix string) bool {
	choices, _ := response["choices"].([]any)
	for _, ch := range choices {
		choice, _ := ch.(map[string]any)
		if choice == nil {
			continue
		}
		message, _ := choice["message"].(map[string]any)
		if message == nil {
			continue
		}
		content, _ := message["content"].(string)
		message["content"] = prefix + content
		return true
	}
	return false
}

func FoldReasoningIntoContent(response map[string]any, collapsible bool) {
	blockStart := streamThinkingBlockStart(collapsible)
	blockEnd := streamThinkingBlockEnd(collapsible)
	choices, _ := response["choices"].([]any)
	for _, ch := range choices {
		choice, _ := ch.(map[string]any)
		if choice == nil {
			continue
		}
		message, _ := choice["message"].(map[string]any)
		if message == nil {
			continue
		}
		reasoning, ok := message["reasoning_content"].(string)
		if !ok || reasoning == "" {
			continue
		}
		content, _ := message["content"].(string)
		message["content"] = blockStart + reasoning + blockEnd + content
	}
}

func streamThinkingBlockStart(collapsible bool) string {
	if collapsible {
		return "<details open>\n<summary>Thinking</summary>\n\n"
	}
	return "<think>\n"
}

func streamThinkingBlockEnd(collapsible bool) string {
	if collapsible {
		return "\n</details>\n\n"
	}
	return "\n</think>\n\n"
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// RecoveryNoticeChunk builds an SSE chunk for recovery notice injection.
func RecoveryNoticeChunk(model, notice string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-dsproxy-recovery",
		"object":  "chat.completion.chunk",
		"created": float64(0),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         map[string]any{"content": notice},
				"finish_reason": nil,
			},
		},
	}
}

func InjectRecoveryNotice(chunk map[string]any, notice string) bool {
	choices, _ := chunk["choices"].([]any)
	for _, ch := range choices {
		choice, _ := ch.(map[string]any)
		if choice == nil {
			continue
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if _, hasContent := delta["content"]; !hasContent && delta["tool_calls"] == nil {
			continue
		}
		existing, _ := delta["content"].(string)
		delta["content"] = notice + existing
		return true
	}
	return false
}
