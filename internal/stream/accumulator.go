package stream

import (
	"fmt"

	"dsproxy/internal/reasoning"
)

type Choice struct {
	Role                string
	Content             string
	ReasoningContent    string
	HasReasoningContent bool
	ToolCalls           []map[string]any
	FinishReason        string
}

type Accumulator struct {
	choices       map[int]*Choice
	storedChoices map[string]string
}

func NewAccumulator() *Accumulator {
	return &Accumulator{
		choices:       map[int]*Choice{},
		storedChoices: map[string]string{},
	}
}

func (a *Accumulator) IngestChunk(chunk map[string]any) {
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
		c := a.choice(index)
		if fr, ok := choice["finish_reason"].(string); ok {
			c.FinishReason = fr
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if role, ok := delta["role"].(string); ok && role != "" {
			c.Role = role
		}
		if content, ok := delta["content"].(string); ok {
			c.Content += content
		}
		if rc, ok := delta["reasoning_content"].(string); ok {
			c.HasReasoningContent = true
			c.ReasoningContent += rc
		}
		a.mergeToolCallDeltas(c, delta["tool_calls"])
	}
}

func (a *Accumulator) choice(index int) *Choice {
	if c, ok := a.choices[index]; ok {
		return c
	}
	c := &Choice{Role: "assistant"}
	a.choices[index] = c
	return c
}

func (a *Accumulator) mergeToolCallDeltas(c *Choice, deltas any) {
	list, ok := deltas.([]any)
	if !ok {
		return
	}
	for _, raw := range list {
		d, _ := raw.(map[string]any)
		if d == nil {
			continue
		}
		index := len(c.ToolCalls)
		if v, ok := d["index"].(float64); ok {
			index = int(v)
		}
		for len(c.ToolCalls) <= index {
			c.ToolCalls = append(c.ToolCalls, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": "", "arguments": "",
				},
			})
		}
		tc := c.ToolCalls[index]
		if id, ok := d["id"]; ok {
			tc["id"] = id
		}
		if typ, ok := d["type"]; ok {
			tc["type"] = typ
		}
		fnDelta, _ := d["function"].(map[string]any)
		if fnDelta == nil {
			continue
		}
		fn, _ := tc["function"].(map[string]any)
		if fn == nil {
			fn = map[string]any{"name": "", "arguments": ""}
			tc["function"] = fn
		}
		if name, ok := fnDelta["name"].(string); ok && name != "" {
			existing, _ := fn["name"].(string)
			if existing == "" {
				fn["name"] = name
			} else {
				fn["name"] = existing + name
			}
		}
		if args, ok := fnDelta["arguments"]; ok && args != nil {
			prev, _ := fn["arguments"].(string)
			fn["arguments"] = prev + fmt.Sprint(args)
		}
	}
}

func (c *Choice) ToMessage() map[string]any {
	msg := map[string]any{
		"role":    c.Role,
		"content": c.Content,
	}
	if c.HasReasoningContent {
		msg["reasoning_content"] = c.ReasoningContent
	}
	if len(c.ToolCalls) > 0 {
		msg["tool_calls"] = toAnySlice(c.ToolCalls)
	}
	return msg
}

func (a *Accumulator) Messages() []map[string]any {
	indices := make([]int, 0, len(a.choices))
	for i := range a.choices {
		indices = append(indices, i)
	}
	sortInts(indices)
	var out []map[string]any
	for _, i := range indices {
		out = append(out, a.choices[i].ToMessage())
	}
	return out
}

func (a *Accumulator) StoreReasoning(store *reasoning.Store, scope, cacheNamespace string, prior []map[string]any) int {
	stored := 0
	for index, c := range a.choices {
		stored += a.storeChoice(index, c, store, scope, "final", cacheNamespace, prior)
	}
	return stored
}

func (a *Accumulator) StoreReadyReasoning(store *reasoning.Store, scope, cacheNamespace string, prior []map[string]any) int {
	stored := 0
	for index, c := range a.choices {
		if c.FinishReason != "" {
			stored += a.storeChoice(index, c, store, scope, "final", cacheNamespace, prior)
		} else if a.hasIdentifiedToolCalls(c) {
			stored += a.storeChoice(index, c, store, scope, "tool_call", cacheNamespace, prior)
		}
	}
	return stored
}

func (a *Accumulator) storeChoice(index int, c *Choice, store *reasoning.Store, scope, stage, cacheNamespace string, prior []map[string]any) int {
	rank := map[string]int{"tool_call": 1, "final": 2}
	key := fmt.Sprintf("%d:%s", index, scope)
	if rank[a.storedChoices[key]] >= rank[stage] {
		return 0
	}
	n := store.StoreAssistantMessage(c.ToMessage(), scope, cacheNamespace, prior)
	if n > 0 {
		a.storedChoices[key] = stage
	}
	return n
}

func (a *Accumulator) hasIdentifiedToolCalls(c *Choice) bool {
	if !c.HasReasoningContent || len(c.ToolCalls) == 0 {
		return false
	}
	for _, tc := range c.ToolCalls {
		if tc["id"] == nil || fmt.Sprint(tc["id"]) == "" {
			return false
		}
	}
	return true
}

func sortInts(v []int) {
	for i := 0; i < len(v); i++ {
		for j := i + 1; j < len(v); j++ {
			if v[j] < v[i] {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
}

func toAnySlice(in []map[string]any) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
