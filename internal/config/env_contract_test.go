package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvContract ensures that every key in .env.example appears in the
// ApplicationEnvVars allowlist. This guards against drift between documentation
// and code.
func TestEnvContract(t *testing.T) {
	// Locate .env.example relative to the module root.
	// The test runs from internal/config/ so walk up two levels.
	cwd, err := os.Getwd()
	if err != nil {
		t.Skipf("cannot get cwd: %v", err)
	}
	examplePath := findEnvExample(cwd)
	if examplePath == "" {
		t.Skip(".env.example not found; skipping env contract test")
	}

	data, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", examplePath, err)
	}

	lines := strings.Split(string(data), "\n")
	exampleKeys := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Lines are KEY=VALUE or KEY= (empty value).
		if idx := strings.Index(line, "="); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			if key != "" {
				exampleKeys[key] = true
			}
		}
	}

	allowlist := make(map[string]bool)
	for _, k := range ApplicationEnvVars {
		allowlist[k] = true
	}

	// Every key in .env.example must be in the allowlist.
	for k := range exampleKeys {
		if !allowlist[k] {
			t.Errorf(".env.example key %q not found in ApplicationEnvVars allowlist", k)
		}
	}

	// Every key in the allowlist that is user-facing should appear in .env.example.
	// We skip HOST, PORT (explained in comments) and HOST_PORT (Docker-only).
	optionalInExample := map[string]bool{
		EnvHost:     true,
		EnvPort:     true,
		EnvHostPort: true,
	}
	for k := range allowlist {
		if optionalInExample[k] {
			continue
		}
		if !exampleKeys[k] {
			t.Errorf("ApplicationEnvVars key %q not found in .env.example", k)
		}
	}
}

func findEnvExample(cwd string) string {
	for range 5 {
		p := filepath.Join(cwd, ".env.example")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return ""
}

// TestEnvContractRoundTrip verifies that calling fromEnv after os.Setenv
// for every key listed in ApplicationEnvVars (set to a known-safe value)
// does not panic and produces a config where Validate passes.
func TestEnvContractRoundTrip(t *testing.T) {
	for _, key := range ApplicationEnvVars {
		t.Setenv(key, safeValue(key))
	}
	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("fromEnv failed: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate failed with safe values: %v", err)
	}
}

func safeValue(key string) string {
	switch key {
	case EnvHost:
		return "localhost"
	case EnvPort:
		return "8080"
	case EnvHostPort:
		return "8080"
	case EnvBaseURL:
		return "https://api.example.com"
	case EnvModel:
		return "test-model"
	case EnvThinking:
		return "enabled"
	case EnvReasoningEffort:
		return "low"
	case EnvDisplayReasoning:
		return "true"
	case EnvCollapsibleReasoning:
		return "false"
	case EnvVerbose:
		return "false"
	case EnvRequestTimeout:
		return "120"
	case EnvMaxRequestBodyBytes:
		return "1048576"
	case EnvCORS:
		return "false"
	case EnvMissingReasoningStrategy:
		return "recover"
	case EnvReasoningContentPath:
		return "/tmp/test-cache.db"
	case EnvReasoningCacheMaxAgeSeconds:
		return "3600"
	case EnvReasoningCacheMaxRows:
		return "1000"
	case EnvClearReasoningCache:
		return "false"
	case EnvTraceDir:
		return ""
	default:
		panic(fmt.Sprintf("safeValue: unknown key %q", key))
	}
}
