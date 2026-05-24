package reasoning_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dsproxy/internal/reasoning"
)

func TestStorePruneAndClear(t *testing.T) {
	store, err := reasoning.Open(":memory:", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.Put("a", "reasoning a", map[string]any{"role": "assistant"})
	time.Sleep(2 * time.Millisecond)
	store.Put("b", "reasoning b", map[string]any{"role": "assistant"})
	time.Sleep(2 * time.Millisecond)
	store.Put("c", "reasoning c", map[string]any{"role": "assistant"})

	if _, ok := store.Get("a"); ok {
		t.Fatal("expected key a pruned")
	}
	if v, ok := store.Get("b"); !ok || v != "reasoning b" {
		t.Fatalf("b: got %q ok=%v", v, ok)
	}
	n, err := store.Clear()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("clear count=%d", n)
	}
}

func TestEmptyReasoningStored(t *testing.T) {
	store, err := reasoning.Open(":memory:", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	scope := reasoning.ConversationScope([]map[string]any{{"role": "user", "content": "lookup"}}, "")
	toolCall := map[string]any{
		"id": "call_empty", "type": "function",
		"function": map[string]any{"name": "lookup", "arguments": "{}"},
	}
	message := map[string]any{
		"role": "assistant", "content": "", "reasoning_content": "",
		"tool_calls": []any{toolCall},
	}
	if n := store.StoreAssistantMessage(message, scope, "", nil); n == 0 {
		t.Fatal("expected keys stored")
	}
	if v, ok := store.Get("scope:" + scope + ":tool_call:call_empty"); !ok || v != "" {
		t.Fatalf("empty reasoning: %q ok=%v", v, ok)
	}
	lookup := map[string]any{
		"role": "assistant", "content": "",
		"tool_calls": []any{toolCall},
	}
	if v, ok := store.LookupForMessage(lookup, scope, "", nil); !ok || v != "" {
		t.Fatalf("lookup: %q ok=%v", v, ok)
	}
}

func TestFileStoreCreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "reasoning.sqlite3")
	store, err := reasoning.Open(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
