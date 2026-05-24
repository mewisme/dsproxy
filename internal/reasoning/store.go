package reasoning

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	maxAgeSeconds *int
	maxRows       *int
	path          string
	mu            sync.Mutex
	db            *sql.DB
}

func Open(path string, maxAgeSeconds, maxRows int) (*Store, error) {
	s := &Store{path: path}
	if maxAgeSeconds > 0 {
		s.maxAgeSeconds = &maxAgeSeconds
	}
	if maxRows > 0 {
		s.maxRows = &maxRows
	}
	var err error
	if path == ":memory:" {
		s.db, err = sql.Open("sqlite", ":memory:")
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create reasoning cache dir %s: %w", filepath.Dir(path), err)
		}
		s.db, err = sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("open reasoning cache at %s: %w", path, err)
		}
		_ = os.Chmod(path, 0o600)
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS reasoning_cache (
			key TEXT PRIMARY KEY,
			reasoning TEXT NOT NULL,
			message_json TEXT NOT NULL,
			created_at REAL NOT NULL
		)
	`); err != nil {
		s.db.Close()
		return nil, err
	}
	_, _ = s.Prune()
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) Put(key, reasoning string, message map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return
	}
	msgJSON, _ := json.Marshal(message)
	_, _ = s.db.Exec(`
		INSERT INTO reasoning_cache(key, reasoning, message_json, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			reasoning = excluded.reasoning,
			message_json = excluded.message_json,
			created_at = excluded.created_at
	`, key, reasoning, string(msgJSON), float64(time.Now().UnixNano())/1e9)
	_, _ = s.pruneLocked()
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return "", false
	}
	var reasoning string
	err := s.db.QueryRow(`SELECT reasoning FROM reasoning_cache WHERE key = ?`, key).Scan(&reasoning)
	if err != nil {
		return "", false
	}
	return reasoning, true
}

func (s *Store) StoreAssistantMessage(message map[string]any, scope, cacheNamespace string, priorMessages []map[string]any) int {
	if message["role"] != "assistant" {
		return 0
	}
	reasoning, ok := message["reasoning_content"].(string)
	if !ok {
		return 0
	}
	keys := ScopedReasoningKeys(message, scope)
	if priorMessages != nil {
		keys = append(keys, PortableReasoningKeys(message, cacheNamespace, priorMessages)...)
	}
	keys = uniqueKeys(keys)
	for _, key := range keys {
		s.Put(key, reasoning, message)
	}
	return len(keys)
}

func (s *Store) LookupForMessage(message map[string]any, scope, cacheNamespace string, priorMessages []map[string]any) (string, bool) {
	keys := ScopedReasoningKeys(message, scope)
	if priorMessages != nil {
		keys = append(keys, PortableReasoningKeys(message, cacheNamespace, priorMessages)...)
	}
	for _, key := range keys {
		if r, ok := s.Get(key); ok {
			return r, true
		}
	}
	return "", false
}

func (s *Store) BackfillPortableAliases(message map[string]any, reasoning, cacheNamespace string, priorMessages []map[string]any) int {
	keys := PortableReasoningKeys(message, cacheNamespace, priorMessages)
	if len(keys) == 0 {
		return 0
	}
	withReasoning := copyMap(message)
	withReasoning["reasoning_content"] = reasoning
	count := 0
	for _, key := range uniqueKeys(keys) {
		s.Put(key, reasoning, withReasoning)
		count++
	}
	return count
}

func (s *Store) Clear() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return 0, nil
	}
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM reasoning_cache`).Scan(&count)
	_, err := s.db.Exec(`DELETE FROM reasoning_cache`)
	return count, err
}

func (s *Store) Prune() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pruneLocked()
}

func (s *Store) pruneLocked() (int, error) {
	if s.db == nil {
		return 0, nil
	}
	deleted := 0
	if s.maxAgeSeconds != nil && *s.maxAgeSeconds > 0 {
		cutoff := float64(time.Now().Unix()) - float64(*s.maxAgeSeconds)
		res, err := s.db.Exec(`DELETE FROM reasoning_cache WHERE created_at < ?`, cutoff)
		if err == nil {
			n, _ := res.RowsAffected()
			deleted += int(n)
		}
	}
	if s.maxRows != nil && *s.maxRows > 0 {
		res, err := s.db.Exec(`
			DELETE FROM reasoning_cache
			WHERE key NOT IN (
				SELECT key FROM reasoning_cache
				ORDER BY created_at DESC
				LIMIT ?
			)
		`, *s.maxRows)
		if err == nil {
			n, _ := res.RowsAffected()
			deleted += int(n)
		}
	}
	return deleted, nil
}

func (s *Store) Path() string {
	return s.path
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func MessageJSON(message map[string]any) string {
	b, err := json.Marshal(message)
	if err != nil {
		return fmt.Sprintf("%v", message)
	}
	return string(b)
}
