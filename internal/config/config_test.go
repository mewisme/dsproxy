package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setenv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range ApplicationEnvVars {
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range ApplicationEnvVars {
			os.Unsetenv(k)
		}
	})
}

// --- Defaults ---

func TestDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("fromEnv with no env vars set: %v", err)
	}

	if cfg.Host != DefaultHost {
		t.Errorf("Host: got %q, want %q", cfg.Host, DefaultHost)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port: got %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.UpstreamBaseURL != DefaultUpstreamBaseURL {
		t.Errorf("UpstreamBaseURL: got %q, want %q", cfg.UpstreamBaseURL, DefaultUpstreamBaseURL)
	}
	if cfg.UpstreamModel != DefaultUpstreamModel {
		t.Errorf("UpstreamModel: got %q, want %q", cfg.UpstreamModel, DefaultUpstreamModel)
	}
	if cfg.Thinking != DefaultThinking {
		t.Errorf("Thinking: got %q, want %q", cfg.Thinking, DefaultThinking)
	}
	if cfg.ReasoningEffort != DefaultReasoningEffort {
		t.Errorf("ReasoningEffort: got %q, want %q", cfg.ReasoningEffort, DefaultReasoningEffort)
	}
	if cfg.RequestTimeout != DefaultRequestTimeout {
		t.Errorf("RequestTimeout: got %v, want %v", cfg.RequestTimeout, DefaultRequestTimeout)
	}
	if cfg.MaxRequestBodyBytes != DefaultMaxRequestBodyBytes {
		t.Errorf("MaxRequestBodyBytes: got %d, want %d", cfg.MaxRequestBodyBytes, DefaultMaxRequestBodyBytes)
	}
	if cfg.DisplayReasoning != DefaultDisplayReasoning {
		t.Errorf("DisplayReasoning: got %v, want %v", cfg.DisplayReasoning, DefaultDisplayReasoning)
	}
	if cfg.CollapsibleReasoning != DefaultCollapsibleReasoning {
		t.Errorf("CollapsibleReasoning: got %v, want %v", cfg.CollapsibleReasoning, DefaultCollapsibleReasoning)
	}
	if cfg.CORS != DefaultCORS {
		t.Errorf("CORS: got %v, want %v", cfg.CORS, DefaultCORS)
	}
	if cfg.Verbose != DefaultVerbose {
		t.Errorf("Verbose: got %v, want %v", cfg.Verbose, DefaultVerbose)
	}
	if cfg.MissingReasoningStrategy != DefaultMissingReasoningStrategy {
		t.Errorf("MissingReasoningStrategy: got %q, want %q", cfg.MissingReasoningStrategy, DefaultMissingReasoningStrategy)
	}
	if cfg.ReasoningCacheMaxAgeSeconds != DefaultReasoningCacheMaxAgeSeconds {
		t.Errorf("ReasoningCacheMaxAgeSeconds: got %d, want %d", cfg.ReasoningCacheMaxAgeSeconds, DefaultReasoningCacheMaxAgeSeconds)
	}
	if cfg.ReasoningCacheMaxRows != DefaultReasoningCacheMaxRows {
		t.Errorf("ReasoningCacheMaxRows: got %d, want %d", cfg.ReasoningCacheMaxRows, DefaultReasoningCacheMaxRows)
	}
	if cfg.TraceDir != "" {
		t.Errorf("TraceDir: got %q, want empty", cfg.TraceDir)
	}
	if cfg.ClearReasoningCache {
		t.Errorf("ClearReasoningCache: got true, want false")
	}
	if cfg.ReasoningContentPath == "" {
		t.Errorf("ReasoningContentPath should have a default")
	}

	// Validate on defaults.
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate on defaults should pass: %v", err)
	}
}

// --- Custom values ---

func TestCustomValues(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvHost, "192.168.1.1")
	setenv(t, EnvPort, "8080")
	setenv(t, EnvBaseURL, "https://custom.api.example.com/v1")
	setenv(t, EnvModel, "deepseek-v4-flash")
	setenv(t, EnvThinking, "disabled")
	setenv(t, EnvReasoningEffort, "low")
	setenv(t, EnvRequestTimeout, "60")
	setenv(t, EnvMaxRequestBodyBytes, "1048576")
	setenv(t, EnvDisplayReasoning, "false")
	setenv(t, EnvCollapsibleReasoning, "false")
	setenv(t, EnvCORS, "true")
	setenv(t, EnvVerbose, "true")
	setenv(t, EnvMissingReasoningStrategy, "reject")
	setenv(t, EnvReasoningCacheMaxAgeSeconds, "86400")
	setenv(t, EnvReasoningCacheMaxRows, "50000")
	setenv(t, EnvTraceDir, "/tmp/traces")
	setenv(t, EnvClearReasoningCache, "true")

	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("fromEnv with custom values: %v", err)
	}

	if cfg.Host != "192.168.1.1" {
		t.Errorf("Host: got %q, want 192.168.1.1", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port: got %d, want 8080", cfg.Port)
	}
	if cfg.UpstreamBaseURL != "https://custom.api.example.com/v1" {
		t.Errorf("UpstreamBaseURL: got %q", cfg.UpstreamBaseURL)
	}
	if cfg.UpstreamModel != "deepseek-v4-flash" {
		t.Errorf("UpstreamModel: got %q, want deepseek-v4-flash", cfg.UpstreamModel)
	}
	if cfg.Thinking != "disabled" {
		t.Errorf("Thinking: got %q, want disabled", cfg.Thinking)
	}
	if cfg.ReasoningEffort != "low" {
		t.Errorf("ReasoningEffort: got %q, want low", cfg.ReasoningEffort)
	}
	if cfg.RequestTimeout != 60.0 {
		t.Errorf("RequestTimeout: got %v, want 60", cfg.RequestTimeout)
	}
	if cfg.MaxRequestBodyBytes != 1048576 {
		t.Errorf("MaxRequestBodyBytes: got %d, want 1048576", cfg.MaxRequestBodyBytes)
	}
	if cfg.DisplayReasoning {
		t.Errorf("DisplayReasoning: got true, want false")
	}
	if cfg.CollapsibleReasoning {
		t.Errorf("CollapsibleReasoning: got true, want false")
	}
	if !cfg.CORS {
		t.Errorf("CORS: got false, want true")
	}
	if !cfg.Verbose {
		t.Errorf("Verbose: got false, want true")
	}
	if cfg.MissingReasoningStrategy != "reject" {
		t.Errorf("MissingReasoningStrategy: got %q, want reject", cfg.MissingReasoningStrategy)
	}
	if cfg.ReasoningCacheMaxAgeSeconds != 86400 {
		t.Errorf("ReasoningCacheMaxAgeSeconds: got %d, want 86400", cfg.ReasoningCacheMaxAgeSeconds)
	}
	if cfg.ReasoningCacheMaxRows != 50000 {
		t.Errorf("ReasoningCacheMaxRows: got %d, want 50000", cfg.ReasoningCacheMaxRows)
	}
	if cfg.TraceDir != "/tmp/traces" {
		t.Errorf("TraceDir: got %q, want /tmp/traces", cfg.TraceDir)
	}
	if !cfg.ClearReasoningCache {
		t.Errorf("ClearReasoningCache: got false, want true")
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate on valid custom values: %v", err)
	}
}

// --- Bool variants ---

func TestBoolVariants(t *testing.T) {
	tests := []struct {
		value  string
		expect bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"no", false},
		{"off", false},
	}

	for _, tt := range tests {
		clearEnv(t)
		setenv(t, EnvCORS, tt.value)
		cfg, err := fromEnv(".")
		if err != nil {
			t.Errorf("envBool(%q): unexpected error: %v", tt.value, err)
			continue
		}
		if cfg.CORS != tt.expect {
			t.Errorf("envBool(%q): got %v, want %v", tt.value, cfg.CORS, tt.expect)
		}
	}
}

// --- Invalid values ---

func TestInvalidInt(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvPort, "not-a-number")
	_, err := fromEnv(".")
	if err == nil {
		t.Error("expected error for invalid PORT")
	}
	if !strings.Contains(err.Error(), EnvPort) {
		t.Errorf("error should mention %s: %v", EnvPort, err)
	}
}

func TestInvalidFloat(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvRequestTimeout, "abc")
	_, err := fromEnv(".")
	if err == nil {
		t.Error("expected error for invalid REQUEST_TIMEOUT")
	}
}

func TestInvalidBool(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvCORS, "maybe")
	_, err := fromEnv(".")
	if err == nil {
		t.Error("expected error for invalid CORS")
	}
}

func TestMultipleInvalidValues(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvPort, "bad")
	setenv(t, EnvRequestTimeout, "bad")
	setenv(t, EnvCORS, "bad")
	_, err := fromEnv(".")
	if err == nil {
		t.Fatal("expected error for multiple invalid values")
	}
	msg := err.Error()
	for _, key := range []string{EnvPort, EnvRequestTimeout, EnvCORS} {
		if !strings.Contains(msg, key) {
			t.Errorf("error should mention %s", key)
		}
	}
}

// --- Validate ---

func TestValidatePortRange(t *testing.T) {
	clearEnv(t)
	// Port must be 1-65535. Override from defaults which are valid.
	setenv(t, EnvPort, "0")

	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for port 0")
	}
}

func TestValidateBaseURL(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvBaseURL, "ftp://bad.scheme.com")

	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid URL scheme")
	}
}

func TestValidateThinking(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvThinking, "invalid")

	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid THINKING")
	}
}

func TestValidateReasoningEffort(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvReasoningEffort, "extreme")

	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid REASONING_EFFORT")
	}
}

func TestValidateMissingStrategy(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvMissingReasoningStrategy, "invalid")

	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid strategy")
	}
}

// --- Path resolution ---

func TestEnvPathValueAbsolute(t *testing.T) {
	clearEnv(t)
	abs := filepath.Join(t.TempDir(), "cache.db")
	setenv(t, EnvReasoningContentPath, abs)
	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ReasoningContentPath != abs {
		t.Errorf("got %q, want %q", cfg.ReasoningContentPath, abs)
	}
}

func TestEnvPathValueRelative(t *testing.T) {
	clearEnv(t)
	rel := "data/cache.db"
	setenv(t, EnvReasoningContentPath, rel)
	base := "/base/dir"
	cfg, err := fromEnv(base + string(filepath.Separator) + "dummy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(base, rel)
	if cfg.ReasoningContentPath != expected {
		t.Errorf("got %q, want %q", cfg.ReasoningContentPath, expected)
	}
}

// --- StartupSummary ---

func TestStartupSummaryContainsKeys(t *testing.T) {
	clearEnv(t)
	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("fromEnv: %v", err)
	}
	s := cfg.StartupSummary()
	// Spot-check that important fields appear.
	checks := []string{
		"Listen:",
		"Upstream:",
		DefaultHost,
		DefaultUpstreamBaseURL,
		DefaultUpstreamModel,
		"Thinking:",
		"Reasoning Effort:",
	}
	for _, c := range checks {
		if !strings.Contains(s, c) {
			t.Errorf("StartupSummary missing %q:\n%s", c, s)
		}
	}
}

func TestStartupSummaryTraceDirEmpty(t *testing.T) {
	clearEnv(t)
	cfg, _ := fromEnv(".")
	s := cfg.StartupSummary()
	if strings.Contains(s, "Trace Dir:") {
		t.Error("StartupSummary should not mention trace dir when empty")
	}
}

func TestStartupSummaryTraceDirSet(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvTraceDir, "/traces")
	cfg, _ := fromEnv(".")
	s := cfg.StartupSummary()
	if !strings.Contains(s, "Trace Dir:") || !strings.Contains(s, "/traces") {
		t.Errorf("StartupSummary should mention trace dir:\n%s", s)
	}
}

// --- URL trailing slash trimming ---

func TestBaseURLTrailingSlashTrimmed(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvBaseURL, "https://api.deepseek.com/")
	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UpstreamBaseURL != "https://api.deepseek.com" {
		t.Errorf("trailing slash not trimmed: %q", cfg.UpstreamBaseURL)
	}
}

// --- Normalize thinking ---

func TestNormalizeThinkingPreservesLowerCase(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvThinking, "ENABLED")
	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Thinking != "enabled" {
		t.Errorf("THINKING=ENABLED should normalize to 'enabled', got %q", cfg.Thinking)
	}

	// Validate should pass for valid normalized values.
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate should pass for 'enabled': %v", err)
	}
}

func TestNormalizeThinkingInvalidPreservedForValidation(t *testing.T) {
	clearEnv(t)
	setenv(t, EnvThinking, "bogus")
	cfg, err := fromEnv(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Normalize only lowercases; invalid values are preserved for Validate.
	if cfg.Thinking != "bogus" {
		t.Errorf("normalize should preserve value, got %q", cfg.Thinking)
	}
	// Validate should reject it.
	if err := cfg.Validate(); err == nil {
		t.Error("Validate should reject invalid THINKING value")
	}
}

// --- ApplicationEnvVars completeness ---

func TestApplicationEnvVarsHasAllConstants(t *testing.T) {
	// Every env var name constant should appear in ApplicationEnvVars.
	for _, k := range []string{
		EnvHost, EnvPort, EnvHostPort, EnvBaseURL, EnvModel,
		EnvThinking, EnvReasoningEffort,
		EnvDisplayReasoning, EnvCollapsibleReasoning, EnvVerbose,
		EnvRequestTimeout, EnvMaxRequestBodyBytes, EnvCORS,
		EnvMissingReasoningStrategy, EnvReasoningContentPath,
		EnvReasoningCacheMaxAgeSeconds, EnvReasoningCacheMaxRows,
		EnvClearReasoningCache, EnvTraceDir,
	} {
		found := false
		for _, a := range ApplicationEnvVars {
			if a == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ApplicationEnvVars missing %s", k)
		}
	}
}
