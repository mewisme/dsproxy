package stream

import (
	"sort"
	"time"
)

const (
	thinkingBlockStart            = "<think>\n"
	thinkingBlockEnd              = "\n</think>\n\n"
	collapsibleThinkingBlockStart = "<details open>\n<summary>Thinking</summary>\n\n"
	collapsibleThinkingBlockEnd   = "\n</details>\n\n"
)

type DisplayAdapter struct {
	collapsible   bool
	openChoices   map[int]struct{}
	lastChunkMeta map[string]any
	blockStart    string
	blockEnd      string
}

func NewDisplayAdapter(collapsible bool) *DisplayAdapter {
	d := &DisplayAdapter{
		collapsible:   collapsible,
		openChoices:   map[int]struct{}{},
		lastChunkMeta: map[string]any{},
	}
	if collapsible {
		d.blockStart = collapsibleThinkingBlockStart
		d.blockEnd = collapsibleThinkingBlockEnd
	} else {
		d.blockStart = thinkingBlockStart
		d.blockEnd = thinkingBlockEnd
	}
	return d
}

func (d *DisplayAdapter) RewriteChunk(chunk map[string]any) {
	for _, key := range []string{"id", "object", "created"} {
		if v, ok := chunk[key]; ok {
			d.lastChunkMeta[key] = v
		}
	}
	choices, _ := chunk["choices"].([]any)
	for _, raw := range choices {
		choice, _ := raw.(map[string]any)
		if choice == nil {
			continue
		}
		index := 0
		if v, ok := choice["index"].(float64); ok {
			index = int(v)
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			delta = map[string]any{}
			choice["delta"] = delta
		}
		var mirrored []string
		if rc, ok := delta["reasoning_content"].(string); ok && rc != "" {
			if _, open := d.openChoices[index]; !open {
				mirrored = append(mirrored, d.blockStart)
				d.openChoices[index] = struct{}{}
			}
			mirrored = append(mirrored, rc)
		}
		existingContent, hasContent := delta["content"].(string)
		shouldClose := false
		if _, open := d.openChoices[index]; open {
			shouldClose = hasContent || delta["tool_calls"] != nil || choice["finish_reason"] != nil
		}
		if shouldClose {
			mirrored = append(mirrored, d.blockEnd)
			delete(d.openChoices, index)
		}
		if len(mirrored) == 0 {
			continue
		}
		if hasContent {
			mirrored = append(mirrored, existingContent)
		}
		delta["content"] = concat(mirrored)
	}
}

func (d *DisplayAdapter) FlushChunk(model string) map[string]any {
	if len(d.openChoices) == 0 {
		return nil
	}
	indices := make([]int, 0, len(d.openChoices))
	for i := range d.openChoices {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	var choices []any
	for _, index := range indices {
		choices = append(choices, map[string]any{
			"index":         index,
			"delta":         map[string]any{"content": d.blockEnd},
			"finish_reason": nil,
		})
	}
	d.openChoices = map[int]struct{}{}
	created := time.Now().Unix()
	if v, ok := d.lastChunkMeta["created"]; ok {
		created = toInt64(v)
	}
	return map[string]any{
		"id":      orMeta(d.lastChunkMeta, "id", "chatcmpl-reasoning-close"),
		"object":  orMeta(d.lastChunkMeta, "object", "chat.completion.chunk"),
		"created": float64(created),
		"model":   model,
		"choices": choices,
	}
}

func concat(parts []string) string {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return string(b)
}

func orMeta(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		return fmtString(v)
	}
	return def
}

func fmtString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	default:
		return ""
	}
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return time.Now().Unix()
	}
}
