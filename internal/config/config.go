package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	AppDirName               = ".dsproxy"
	EnvFileName              = ".env"
	ReasoningContentFileName = "reasoning_content.sqlite3"

	DefaultHost                        = "127.0.0.1"
	DefaultPort                        = 9999
	DefaultUpstreamBaseURL             = "https://api.deepseek.com"
	DefaultUpstreamModel               = "deepseek-v4-pro"
	DefaultThinking                    = "enabled"
	DefaultReasoningEffort             = "max"
	DefaultDisplayReasoning            = true
	DefaultCollapsibleReasoning        = true
	DefaultVerbose                     = false
	DefaultRequestTimeout              = 300.0
	DefaultMaxRequestBodyBytes         = 20 * 1024 * 1024
	DefaultCORS                        = false
	DefaultMissingReasoningStrategy    = "recover"
	DefaultReasoningCacheMaxAgeSeconds = 30 * 24 * 60 * 60
	DefaultReasoningCacheMaxRows       = 100_000
)

type ProxyConfig struct {
	Host                        string
	Port                        int
	UpstreamBaseURL             string
	UpstreamModel               string
	Thinking                    string
	ReasoningEffort             string
	RequestTimeout              float64
	MaxRequestBodyBytes         int
	ReasoningContentPath        string
	MissingReasoningStrategy    string
	ReasoningCacheMaxAgeSeconds int
	ReasoningCacheMaxRows       int
	DisplayReasoning            bool
	CollapsibleReasoning        bool
	CORS                        bool
	Verbose                     bool
	TraceDir                    string
	ClearReasoningCache         bool
	EnvPath                     string
}

func DefaultAppDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return AppDirName
	}
	return filepath.Join(home, AppDirName)
}

func DefaultReasoningContentPath() string {
	return filepath.Join(DefaultAppDir(), ReasoningContentFileName)
}

func Load() (ProxyConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ProxyConfig{}, err
	}
	envPath := filepath.Join(cwd, EnvFileName)
	if fileExists(envPath) {
		_ = godotenv.Load(envPath)
	}
	homeEnv := filepath.Join(DefaultAppDir(), EnvFileName)
	if homeEnv != envPath && fileExists(homeEnv) {
		_ = godotenv.Load(homeEnv)
	}
	return fromEnv(envPath), nil
}

func fromEnv(envPath string) ProxyConfig {
	baseDir := filepath.Dir(envPath)
	return ProxyConfig{
		Host:                        envStr("HOST", DefaultHost),
		Port:                        envInt("PORT", DefaultPort),
		UpstreamBaseURL:             strings.TrimRight(envStr("BASE_URL", DefaultUpstreamBaseURL), "/"),
		UpstreamModel:               envStr("MODEL", DefaultUpstreamModel),
		Thinking:                    normalizeThinking(envStr("THINKING", DefaultThinking)),
		ReasoningEffort:             envStr("REASONING_EFFORT", DefaultReasoningEffort),
		RequestTimeout:              envFloat("REQUEST_TIMEOUT", DefaultRequestTimeout),
		MaxRequestBodyBytes:         envInt("MAX_REQUEST_BODY_BYTES", DefaultMaxRequestBodyBytes),
		ReasoningContentPath:        envPathValue("REASONING_CONTENT_PATH", DefaultReasoningContentPath(), baseDir),
		MissingReasoningStrategy:    normalizeMissingReasoningStrategy(envStr("MISSING_REASONING_STRATEGY", DefaultMissingReasoningStrategy)),
		ReasoningCacheMaxAgeSeconds: envInt("REASONING_CACHE_MAX_AGE_SECONDS", DefaultReasoningCacheMaxAgeSeconds),
		ReasoningCacheMaxRows:       envInt("REASONING_CACHE_MAX_ROWS", DefaultReasoningCacheMaxRows),
		DisplayReasoning:            envBool("DISPLAY_REASONING", DefaultDisplayReasoning),
		CollapsibleReasoning:        envBool("COLLAPSIBLE_REASONING", DefaultCollapsibleReasoning),
		CORS:                        envBool("CORS", DefaultCORS),
		Verbose:                     envBool("VERBOSE", DefaultVerbose),
		TraceDir:                    envStr("TRACE_DIR", ""),
		ClearReasoningCache:         envBool("CLEAR_REASONING_CACHE", false),
		EnvPath:                     envPath,
	}
}

func envStr(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func envFloat(key string, defaultVal float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

func envBool(key string, defaultVal bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultVal
	}
}

func envPathValue(key, defaultPath, relativeBase string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultPath
	}
	if filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(relativeBase, v)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func normalizeThinking(v string) string {
	thinking := strings.ToLower(strings.TrimSpace(v))
	if thinking == "enabled" || thinking == "disabled" {
		return thinking
	}
	return DefaultThinking
}

func normalizeMissingReasoningStrategy(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "recover" || s == "reject" {
		return s
	}
	return DefaultMissingReasoningStrategy
}
