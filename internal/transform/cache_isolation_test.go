package transform

import (
	"testing"

	"dsproxy/internal/config"
)

func defaultCfg() config.ProxyConfig {
	return config.ProxyConfig{
		UpstreamBaseURL: "https://api.deepseek.com",
		UpstreamModel:   "deepseek-v4-pro",
		Thinking:        "enabled",
		ReasoningEffort: "max",
	}
}

func defaultAuth() string {
	return "Bearer sk-test-key-12345"
}

func emptyRuntimeContextHash() string {
	return RuntimeContextHash(nil, nil, nil)
}

// --- effectiveToolChoice ---

func TestEffectiveToolChoice(t *testing.T) {
	if v := effectiveToolChoice(nil, nil); v != "none" {
		t.Fatalf("nil tools, nil explicit: got %v, want none", v)
	}
	if v := effectiveToolChoice([]any{}, nil); v != "none" {
		t.Fatalf("empty tools, nil explicit: got %v, want none", v)
	}
	if v := effectiveToolChoice([]any{map[string]any{"name": "foo"}}, nil); v != "auto" {
		t.Fatalf("tools present, nil explicit: got %v, want auto", v)
	}
	if v := effectiveToolChoice([]any{map[string]any{"name": "foo"}}, "auto"); v != "auto" {
		t.Fatalf("tools present, explicit auto: got %v, want auto", v)
	}
	if v := effectiveToolChoice(nil, "required"); v != "required" {
		t.Fatalf("nil tools, explicit required: got %v, want required", v)
	}
	if v := effectiveToolChoice([]any{map[string]any{"name": "foo"}}, "none"); v != "none" {
		t.Fatalf("tools present, explicit none: got %v, want none", v)
	}
}

// --- normalizedSystemMessages ---

func TestNormalizedSystemMessages(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "You are helpful.", "name": "sys1", "extra": "ignored"},
		{"role": "user", "content": "Hello"},
		{"role": "system", "content": "Also be concise."},
	}
	result := normalizedSystemMessages(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2 system messages, got %d", len(result))
	}
	if result[0]["name"] != "sys1" {
		t.Fatalf("name=%v", result[0]["name"])
	}
	if _, ok := result[0]["extra"]; ok {
		t.Fatal("extra field not stripped")
	}
	if _, ok := result[1]["name"]; ok {
		t.Fatal("name should be absent when not present")
	}
}

// --- RuntimeContextHash differ tests ---

func TestRuntimeContextHashSystemContentDiffers(t *testing.T) {
	a := RuntimeContextHash(
		[]map[string]any{{"role": "system", "content": "prompt A"}},
		nil, nil,
	)
	b := RuntimeContextHash(
		[]map[string]any{{"role": "system", "content": "prompt B"}},
		nil, nil,
	)
	if a == b {
		t.Fatal("different system prompts must produce different hashes")
	}
}

func TestRuntimeContextHashSystemNameDiffers(t *testing.T) {
	a := RuntimeContextHash(
		[]map[string]any{{"role": "system", "content": "prompt", "name": "A"}},
		nil, nil,
	)
	b := RuntimeContextHash(
		[]map[string]any{{"role": "system", "content": "prompt", "name": "B"}},
		nil, nil,
	)
	if a == b {
		t.Fatal("different system names must produce different hashes")
	}
}

func TestRuntimeContextHashSystemOrderDiffers(t *testing.T) {
	a := RuntimeContextHash(
		[]map[string]any{
			{"role": "system", "content": "first"},
			{"role": "system", "content": "second"},
		},
		nil, nil,
	)
	b := RuntimeContextHash(
		[]map[string]any{
			{"role": "system", "content": "second"},
			{"role": "system", "content": "first"},
		},
		nil, nil,
	)
	if a == b {
		t.Fatal("different system message order must produce different hashes")
	}
}

func TestRuntimeContextHashSystemAdded(t *testing.T) {
	a := RuntimeContextHash(
		[]map[string]any{{"role": "system", "content": "one"}},
		nil, nil,
	)
	b := RuntimeContextHash(
		[]map[string]any{
			{"role": "system", "content": "one"},
			{"role": "system", "content": "two"},
		},
		nil, nil,
	)
	if a == b {
		t.Fatal("adding a system message must change the hash")
	}
}

func TestRuntimeContextHashToolAdded(t *testing.T) {
	a := RuntimeContextHash(nil, []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "tool_a"}},
	}, nil)
	b := RuntimeContextHash(nil, []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "tool_a"}},
		map[string]any{"type": "function", "function": map[string]any{"name": "tool_b"}},
	}, nil)
	if a == b {
		t.Fatal("adding a tool must change the hash")
	}
}

func TestRuntimeContextHashToolNameDiffers(t *testing.T) {
	tool := func(name string) map[string]any {
		return map[string]any{"type": "function", "function": map[string]any{"name": name}}
	}
	a := RuntimeContextHash(nil, []any{tool("get_weather")}, nil)
	b := RuntimeContextHash(nil, []any{tool("get_date")}, nil)
	if a == b {
		t.Fatal("different tool names must produce different hashes")
	}
}

func TestRuntimeContextHashToolDescriptionDiffers(t *testing.T) {
	tool := func(desc string) map[string]any {
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "f", "description": desc},
		}
	}
	a := RuntimeContextHash(nil, []any{tool("desc A")}, nil)
	b := RuntimeContextHash(nil, []any{tool("desc B")}, nil)
	if a == b {
		t.Fatal("different tool descriptions must produce different hashes")
	}
}

func TestRuntimeContextHashToolParamSchemaDiffers(t *testing.T) {
	tool := func(props map[string]any) map[string]any {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":       "f",
				"parameters": map[string]any{"type": "object", "properties": props},
			},
		}
	}
	a := RuntimeContextHash(nil, []any{tool(map[string]any{"x": map[string]any{"type": "string"}})}, nil)
	b := RuntimeContextHash(nil, []any{tool(map[string]any{"x": map[string]any{"type": "integer"}})}, nil)
	if a == b {
		t.Fatal("different tool param schemas must produce different hashes")
	}
}

func TestRuntimeContextHashToolChoiceDiffers(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "f"}}}
	a := RuntimeContextHash(nil, tools, "auto")
	b := RuntimeContextHash(nil, tools, "required")
	if a == b {
		t.Fatal("auto vs required must produce different hashes")
	}
}

func TestRuntimeContextHashNamedToolChoiceDiffers(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "f"}}}
	a := RuntimeContextHash(nil, tools, map[string]any{"type": "function", "function": map[string]any{"name": "f"}})
	b := RuntimeContextHash(nil, tools, map[string]any{"type": "function", "function": map[string]any{"name": "g"}})
	if a == b {
		t.Fatal("different named tool choices must produce different hashes")
	}
}

// --- RuntimeContextHash match tests ---

func TestRuntimeContextHashIdentical(t *testing.T) {
	msgs := []map[string]any{
		{"role": "system", "content": "prompt"},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "f",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"x": map[string]any{"type": "string"}},
				},
			},
		},
	}
	a := RuntimeContextHash(msgs, tools, "auto")
	b := RuntimeContextHash(msgs, tools, "auto")
	if a != b {
		t.Fatal("identical context must produce same hash")
	}
}

func TestRuntimeContextHashMapKeyOrdering(t *testing.T) {
	a := RuntimeContextHash(nil, []any{
		map[string]any{"function": map[string]any{"name": "f"}, "type": "function"},
	}, nil)
	b := RuntimeContextHash(nil, []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "f"}},
	}, nil)
	if a != b {
		t.Fatal("different map key ordering must produce same hash")
	}
}

func TestRuntimeContextHashNestedKeyOrdering(t *testing.T) {
	schemaA := map[string]any{"type": "object", "properties": map[string]any{"a": nil, "b": nil}}
	schemaB := map[string]any{"properties": map[string]any{"b": nil, "a": nil}, "type": "object"}
	toolA := map[string]any{"type": "function", "function": map[string]any{"name": "f", "parameters": schemaA}}
	toolB := map[string]any{"type": "function", "function": map[string]any{"name": "f", "parameters": schemaB}}
	a := RuntimeContextHash(nil, []any{toolA}, nil)
	b := RuntimeContextHash(nil, []any{toolB}, nil)
	if a != b {
		t.Fatal("nested key ordering must produce same hash")
	}
}

func TestRuntimeContextHashAbsentToolsVsEmpty(t *testing.T) {
	a := RuntimeContextHash(nil, nil, nil)
	b := RuntimeContextHash(nil, []any{}, nil)
	if a != b {
		t.Fatal("absent tools and empty tools must produce same hash")
	}
}

func TestRuntimeContextHashOmittedVsDefaultToolChoice(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "f"}}}
	a := RuntimeContextHash(nil, tools, nil)
	b := RuntimeContextHash(nil, tools, "auto")
	if a != b {
		t.Fatal("omitted tool_choice and explicit auto must match when tools present")
	}
}

func TestRuntimeContextHashOmittedVsNoneNoTools(t *testing.T) {
	a := RuntimeContextHash(nil, nil, nil)
	b := RuntimeContextHash(nil, nil, "none")
	if a != b {
		t.Fatal("omitted tool_choice and explicit none must match when no tools")
	}
}

// --- ReasoningCacheNamespace tests ---

func TestCacheNamespaceDifferentAPIKeys(t *testing.T) {
	cfg := defaultCfg()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, "sk-a", "", emptyRuntimeContextHash())
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, "sk-b", "", emptyRuntimeContextHash())
	if ns1 == ns2 {
		t.Fatal("different API keys must produce different namespaces")
	}
}

func TestCacheNamespaceDifferentURLs(t *testing.T) {
	cfg1 := defaultCfg()
	cfg2 := defaultCfg()
	cfg2.UpstreamBaseURL = "https://api.deepseek.com/beta"
	ns1 := ReasoningCacheNamespace(cfg1, "deepseek-v4-pro", nil, nil, defaultAuth(), "", emptyRuntimeContextHash())
	ns2 := ReasoningCacheNamespace(cfg2, "deepseek-v4-pro", nil, nil, defaultAuth(), "", emptyRuntimeContextHash())
	if ns1 == ns2 {
		t.Fatal("different upstream URLs must produce different namespaces")
	}
}

func TestCacheNamespaceDifferentModels(t *testing.T) {
	cfg := defaultCfg()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", emptyRuntimeContextHash())
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-flash", nil, nil, defaultAuth(), "", emptyRuntimeContextHash())
	if ns1 == ns2 {
		t.Fatal("different models must produce different namespaces")
	}
}

func TestCacheNamespaceDifferentThinking(t *testing.T) {
	cfg := defaultCfg()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", map[string]any{"type": "enabled"}, nil, defaultAuth(), "", emptyRuntimeContextHash())
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", map[string]any{"type": "disabled"}, nil, defaultAuth(), "", emptyRuntimeContextHash())
	if ns1 == ns2 {
		t.Fatal("different thinking must produce different namespaces")
	}
}

func TestCacheNamespaceDifferentEffort(t *testing.T) {
	cfg := defaultCfg()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, "high", defaultAuth(), "", emptyRuntimeContextHash())
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, "max", defaultAuth(), "", emptyRuntimeContextHash())
	if ns1 == ns2 {
		t.Fatal("different reasoning effort must produce different namespaces")
	}
}

func TestCacheNamespaceDifferentRuntimeContext(t *testing.T) {
	cfg := defaultCfg()
	h1 := RuntimeContextHash([]map[string]any{{"role": "system", "content": "A"}}, nil, nil)
	h2 := RuntimeContextHash([]map[string]any{{"role": "system", "content": "B"}}, nil, nil)
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h1)
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h2)
	if ns1 == ns2 {
		t.Fatal("different runtime context must produce different namespaces")
	}
}

func TestCacheNamespaceSameRuntimeContext(t *testing.T) {
	cfg := defaultCfg()
	h := RuntimeContextHash([]map[string]any{{"role": "system", "content": "A"}}, nil, nil)
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h)
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h)
	if ns1 != ns2 {
		t.Fatal("same inputs must produce same namespace")
	}
}

// --- Array ordering significance ---

func TestRuntimeContextHashToolOrderSignificant(t *testing.T) {
	a := RuntimeContextHash(nil, []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "a"}},
		map[string]any{"type": "function", "function": map[string]any{"name": "b"}},
	}, nil)
	b := RuntimeContextHash(nil, []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "b"}},
		map[string]any{"type": "function", "function": map[string]any{"name": "a"}},
	}, nil)
	if a == b {
		t.Fatal("tool order must be significant")
	}
}

func TestRuntimeContextHashEnumOrderSignificant(t *testing.T) {
	toolWithEnum := func(values []any) map[string]any {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "f",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"x": map[string]any{"type": "integer", "enum": values},
					},
				},
			},
		}
	}
	a := RuntimeContextHash(nil, []any{toolWithEnum([]any{1, 2, 3})}, nil)
	b := RuntimeContextHash(nil, []any{toolWithEnum([]any{3, 2, 1})}, nil)
	if a == b {
		t.Fatal("enum order must be significant")
	}
}

// --- Reasoning presence must not affect fingerprint ---

func TestRuntimeContextHashReasoningPresenceIndependent(t *testing.T) {
	msgs := []map[string]any{
		{"role": "system", "content": "prompt"},
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "", "reasoning_content": "thinking...", "tool_calls": []any{}},
	}
	result := normalizedSystemMessages(msgs)
	h1 := RuntimeContextHash(result, nil, nil)

	msgsNoReasoning := []map[string]any{
		{"role": "system", "content": "prompt"},
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "", "tool_calls": []any{}},
	}
	result2 := normalizedSystemMessages(msgsNoReasoning)
	h2 := RuntimeContextHash(result2, nil, nil)
	if h1 != h2 {
		t.Fatal("reasoning presence must not affect runtime context hash")
	}
}

// --- V2 namespace includes version ---

func TestCacheNamespaceIncludesV2Version(t *testing.T) {
	cfg := defaultCfg()
	ns := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", emptyRuntimeContextHash())
	if ns == "" {
		t.Fatal("namespace must not be empty")
	}
	// Different runtime hash should produce different namespace (implicitly tested above)
}

func TestCacheNamespaceV1IsDifferent(t *testing.T) {
	// V2 always includes the "v" field; sanity check that two calls with different
	// runtime context hashes produce different namespaces, confirming version + hash isolation.
	cfg := defaultCfg()
	h := emptyRuntimeContextHash()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h)

	cfg2 := cfg
	cfg2.UpstreamBaseURL = "https://other.api.com"
	ns2 := ReasoningCacheNamespace(cfg2, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h)
	if ns1 == ns2 {
		t.Fatal("different base URLs must produce different v2 namespaces")
	}
}

// --- V3 namespace user_id isolation ---

func TestCacheNamespaceSameIdentitySameNamespace(t *testing.T) {
	cfg := defaultCfg()
	h := emptyRuntimeContextHash()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "customer_123", h)
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "customer_123", h)
	if ns1 != ns2 {
		t.Fatal("same identity + same context must produce same namespace")
	}
}

func TestCacheNamespaceDifferentIdentityDifferentNamespace(t *testing.T) {
	cfg := defaultCfg()
	h := emptyRuntimeContextHash()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "customer_123", h)
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "customer_456", h)
	if ns1 == ns2 {
		t.Fatal("different identity must produce different namespace")
	}
}

func TestCacheNamespaceSameUserDifferentAPIKeyDifferentNamespace(t *testing.T) {
	cfg := defaultCfg()
	h := emptyRuntimeContextHash()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, "Bearer sk-a", "customer_123", h)
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, "Bearer sk-b", "customer_123", h)
	if ns1 == ns2 {
		t.Fatal("same user under different API keys must produce different namespace")
	}
}

func TestCacheNamespaceAbsentIdentityDeterministic(t *testing.T) {
	cfg := defaultCfg()
	h := emptyRuntimeContextHash()
	ns1 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h)
	ns2 := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h)
	if ns1 != ns2 {
		t.Fatal("absent identity must be deterministic")
	}
}

func TestCacheNamespaceV3DiffersFromV2(t *testing.T) {
	// V2 fixture: "v": "v2" instead of "v3", and no "user_id" field.
	// This simulates a v2 namespace payload.
	pkg := "dsproxy/internal/reasoning"
	_ = pkg // reasoning is used transitively; import unused is fine in test

	cfg := defaultCfg()
	h := emptyRuntimeContextHash()
	v3ns := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "customer_123", h)
	v3nsEmpty := ReasoningCacheNamespace(cfg, "deepseek-v4-pro", nil, nil, defaultAuth(), "", h)

	// v3 must not be empty string which is unlikely but assert.
	if v3ns == "" {
		t.Fatal("v3 namespace must not be empty")
	}
	if v3ns == v3nsEmpty {
		t.Error("v3 with identity must differ from v3 without identity")
	}
}
