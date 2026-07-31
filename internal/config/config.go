package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Env var name constants
const (
	EnvHost                        = "HOST"
	EnvPort                        = "PORT"
	EnvHostPort                    = "HOST_PORT"
	EnvBaseURL                     = "BASE_URL"
	EnvModel                       = "MODEL"
	EnvThinking                    = "THINKING"
	EnvReasoningEffort             = "REASONING_EFFORT"
	EnvDisplayReasoning            = "DISPLAY_REASONING"
	EnvCollapsibleReasoning        = "COLLAPSIBLE_REASONING"
	EnvVerbose                     = "VERBOSE"
	EnvRequestTimeout              = "REQUEST_TIMEOUT"
	EnvMaxRequestBodyBytes         = "MAX_REQUEST_BODY_BYTES"
	EnvCORS                        = "CORS"
	EnvMissingReasoningStrategy    = "MISSING_REASONING_STRATEGY"
	EnvReasoningContentPath        = "REASONING_CONTENT_PATH"
	EnvReasoningCacheMaxAgeSeconds = "REASONING_CACHE_MAX_AGE_SECONDS"
	EnvReasoningCacheMaxRows       = "REASONING_CACHE_MAX_ROWS"
	EnvClearReasoningCache         = "CLEAR_REASONING_CACHE"
	EnvTraceDir                    = "TRACE_DIR"
	EnvNgrokEnabled                = "NGROK_ENABLED"
	EnvNgrokAuthtoken              = "NGROK_AUTHTOKEN"
	EnvNgrokURL                    = "NGROK_URL"
)

// ApplicationEnvVars is the allowlist of all env vars this application reads.
var ApplicationEnvVars = []string{
	EnvHost,
	EnvPort,
	EnvHostPort,
	EnvBaseURL,
	EnvModel,
	EnvThinking,
	EnvReasoningEffort,
	EnvDisplayReasoning,
	EnvCollapsibleReasoning,
	EnvVerbose,
	EnvRequestTimeout,
	EnvMaxRequestBodyBytes,
	EnvCORS,
	EnvMissingReasoningStrategy,
	EnvReasoningContentPath,
	EnvReasoningCacheMaxAgeSeconds,
	EnvReasoningCacheMaxRows,
	EnvClearReasoningCache,
	EnvTraceDir,
	EnvNgrokEnabled,
	EnvNgrokAuthtoken,
	EnvNgrokURL,
}

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
	NgrokEnabled                bool
	NgrokAuthtoken              string
	NgrokURL                    string
	EnvPath                     string
	LoadedEnvFiles              []string
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

	projectEnvPath := filepath.Join(cwd, EnvFileName)
	homeEnvPath := filepath.Join(DefaultAppDir(), EnvFileName)

	merged, loadedFiles, err := mergeEnvFiles(projectEnvPath, homeEnvPath)
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("loading env files: %w", err)
	}

	for k, v := range merged {
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}

	cfg, err := fromEnv(projectEnvPath)
	cfg.LoadedEnvFiles = loadedFiles
	return cfg, err
}

func mergeEnvFiles(projectPath, homePath string) (map[string]string, []string, error) {
	var loadedFiles []string
	merged := make(map[string]string)

	// Home .env (lower priority)
	if projectPath != homePath {
		homeMap, err := godotenv.Read(homePath)
		if err == nil {
			for k, v := range homeMap {
				merged[k] = v
			}
			loadedFiles = append(loadedFiles, homePath)
		}
	}

	// Project .env (higher priority, overrides home)
	projectMap, err := godotenv.Read(projectPath)
	if err == nil {
		for k, v := range projectMap {
			merged[k] = v
		}
		loadedFiles = append(loadedFiles, projectPath)
	}

	return merged, loadedFiles, nil
}

func fromEnv(envPath string) (ProxyConfig, error) {
	baseDir := filepath.Dir(envPath)
	var errs []string

	port, err := envInt(EnvPort, DefaultPort)
	if err != nil {
		errs = append(errs, err.Error())
	}

	timeout, err := envFloat(EnvRequestTimeout, DefaultRequestTimeout)
	if err != nil {
		errs = append(errs, err.Error())
	}

	maxBody, err := envInt(EnvMaxRequestBodyBytes, DefaultMaxRequestBodyBytes)
	if err != nil {
		errs = append(errs, err.Error())
	}

	cacheAge, err := envInt(EnvReasoningCacheMaxAgeSeconds, DefaultReasoningCacheMaxAgeSeconds)
	if err != nil {
		errs = append(errs, err.Error())
	}

	cacheRows, err := envInt(EnvReasoningCacheMaxRows, DefaultReasoningCacheMaxRows)
	if err != nil {
		errs = append(errs, err.Error())
	}

	displayReasoning, err := envBool(EnvDisplayReasoning, DefaultDisplayReasoning)
	if err != nil {
		errs = append(errs, err.Error())
	}

	collapsibleReasoning, err := envBool(EnvCollapsibleReasoning, DefaultCollapsibleReasoning)
	if err != nil {
		errs = append(errs, err.Error())
	}

	verbose, err := envBool(EnvVerbose, DefaultVerbose)
	if err != nil {
		errs = append(errs, err.Error())
	}

	cors, err := envBool(EnvCORS, DefaultCORS)
	if err != nil {
		errs = append(errs, err.Error())
	}

	clearCache, err := envBool(EnvClearReasoningCache, false)
	if err != nil {
		errs = append(errs, err.Error())
	}

	ngrokEnabled, err := envBool(EnvNgrokEnabled, false)
	if err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return ProxyConfig{}, fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return ProxyConfig{
		Host:                        envStr(EnvHost, DefaultHost),
		Port:                        port,
		UpstreamBaseURL:             strings.TrimRight(envStr(EnvBaseURL, DefaultUpstreamBaseURL), "/"),
		UpstreamModel:               envStr(EnvModel, DefaultUpstreamModel),
		Thinking:                    normalizeThinking(envStr(EnvThinking, DefaultThinking)),
		ReasoningEffort:             envStr(EnvReasoningEffort, DefaultReasoningEffort),
		RequestTimeout:              timeout,
		MaxRequestBodyBytes:         maxBody,
		ReasoningContentPath:        envPathValue(EnvReasoningContentPath, DefaultReasoningContentPath(), baseDir),
		MissingReasoningStrategy:    normalizeMissingReasoningStrategy(envStr(EnvMissingReasoningStrategy, DefaultMissingReasoningStrategy)),
		ReasoningCacheMaxAgeSeconds: cacheAge,
		ReasoningCacheMaxRows:       cacheRows,
		DisplayReasoning:            displayReasoning,
		CollapsibleReasoning:        collapsibleReasoning,
		CORS:                        cors,
		Verbose:                     verbose,
		TraceDir:                    envStr(EnvTraceDir, ""),
		ClearReasoningCache:         clearCache,
		NgrokEnabled:                ngrokEnabled,
		NgrokAuthtoken:              strings.TrimSpace(os.Getenv(EnvNgrokAuthtoken)),
		NgrokURL:                    normalizeNgrokURL(strings.TrimSpace(os.Getenv(EnvNgrokURL))),
		EnvPath:                     envPath,
	}, nil
}

func envStr(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	return n, nil
}

func envFloat(key string, defaultVal float64) (float64, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", key, v)
	}
	return f, nil
}

func envBool(key string, defaultVal bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal, nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean (true/false, 1/0, yes/no, on/off), got %q", key, v)
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
	return strings.ToLower(strings.TrimSpace(v))
}

func normalizeMissingReasoningStrategy(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func normalizeNgrokURL(v string) string {
	if strings.HasSuffix(v, "/") && !strings.HasSuffix(v, "//") {
		return strings.TrimSuffix(v, "/")
	}
	return v
}

// Validate checks all configuration constraints and returns an error describing
// any problems found.
func (c ProxyConfig) Validate() error {
	var errs []string

	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, fmt.Sprintf("PORT must be between 1 and 65535, got %d", c.Port))
	}

	if c.UpstreamBaseURL == "" {
		errs = append(errs, "BASE_URL must not be empty")
	} else {
		u, err := url.Parse(c.UpstreamBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			errs = append(errs, fmt.Sprintf("BASE_URL must start with http:// or https://, got %q", c.UpstreamBaseURL))
		}
	}

	if c.Thinking != "enabled" && c.Thinking != "disabled" {
		errs = append(errs, fmt.Sprintf("THINKING must be 'enabled' or 'disabled', got %q", c.Thinking))
	}

	validEfforts := map[string]bool{"low": true, "medium": true, "high": true, "max": true}
	if !validEfforts[c.ReasoningEffort] {
		errs = append(errs, fmt.Sprintf("REASONING_EFFORT must be one of: low, medium, high, max, got %q", c.ReasoningEffort))
	}

	if c.RequestTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("REQUEST_TIMEOUT must be positive, got %v", c.RequestTimeout))
	}

	if c.MaxRequestBodyBytes <= 0 {
		errs = append(errs, fmt.Sprintf("MAX_REQUEST_BODY_BYTES must be positive, got %d", c.MaxRequestBodyBytes))
	}

	if c.MissingReasoningStrategy != "recover" && c.MissingReasoningStrategy != "reject" {
		errs = append(errs, fmt.Sprintf("MISSING_REASONING_STRATEGY must be 'recover' or 'reject', got %q", c.MissingReasoningStrategy))
	}

	if c.ReasoningCacheMaxAgeSeconds < 0 {
		errs = append(errs, "REASONING_CACHE_MAX_AGE_SECONDS must not be negative")
	}

	if c.ReasoningCacheMaxRows < 0 {
		errs = append(errs, "REASONING_CACHE_MAX_ROWS must not be negative")
	}

	if c.NgrokEnabled {
		if c.NgrokAuthtoken == "" {
			errs = append(errs, "NGROK_AUTHTOKEN must not be empty when NGROK_ENABLED=true")
		}
		if c.NgrokURL != "" {
			u, err := url.Parse(c.NgrokURL)
			if err != nil || !u.IsAbs() || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(c.NgrokURL, "#") || (u.Path != "" && u.Path != "/") {
				errs = append(errs, "NGROK_URL must be an absolute https URL with a hostname and no path, userinfo, query, or fragment")
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// StartupSummary returns a human-readable multi-line summary of the
// configuration, with no secrets.
func (c ProxyConfig) StartupSummary() string {
	var b strings.Builder
	b.WriteString("Configuration:\n")
	fmt.Fprintf(&b, "  Listen:             %s:%d\n", c.Host, c.Port)
	fmt.Fprintf(&b, "  Upstream:           %s\n", c.UpstreamBaseURL)
	fmt.Fprintf(&b, "  Model:              %s\n", c.UpstreamModel)
	fmt.Fprintf(&b, "  Thinking:           %s\n", c.Thinking)
	fmt.Fprintf(&b, "  Reasoning Effort:   %s\n", c.ReasoningEffort)
	fmt.Fprintf(&b, "  Display Reasoning:  %v\n", c.DisplayReasoning)
	if c.DisplayReasoning {
		fmt.Fprintf(&b, "  Collapsible:        %v\n", c.CollapsibleReasoning)
	}
	fmt.Fprintf(&b, "  CORS:               %v\n", c.CORS)
	fmt.Fprintf(&b, "  Verbose:            %v\n", c.Verbose)
	fmt.Fprintf(&b, "  Request Timeout:    %.0fs\n", c.RequestTimeout)
	fmt.Fprintf(&b, "  Max Body:           %d bytes\n", c.MaxRequestBodyBytes)
	fmt.Fprintf(&b, "  Strategy:           %s\n", c.MissingReasoningStrategy)
	fmt.Fprintf(&b, "  Cache Path:         %s\n", c.ReasoningContentPath)
	fmt.Fprintf(&b, "  Cache Max Age:      %ds\n", c.ReasoningCacheMaxAgeSeconds)
	fmt.Fprintf(&b, "  Cache Max Rows:     %d\n", c.ReasoningCacheMaxRows)
	if c.TraceDir != "" {
		fmt.Fprintf(&b, "  Trace Dir:          %s\n", c.TraceDir)
	}
	if c.ClearReasoningCache {
		fmt.Fprintf(&b, "  Clear Cache:        true (one-shot)\n")
	}
	if !c.NgrokEnabled {
		fmt.Fprintf(&b, "  Ngrok:              disabled\n")
	} else if c.NgrokURL == "" {
		fmt.Fprintf(&b, "  Ngrok:              enabled\n")
		fmt.Fprintf(&b, "  Ngrok Endpoint:     random\n")
	} else {
		fmt.Fprintf(&b, "  Ngrok:              enabled\n")
		fmt.Fprintf(&b, "  Ngrok Endpoint:     %s\n", c.NgrokURL)
	}
	if len(c.LoadedEnvFiles) > 0 {
		b.WriteString("  Env Files:\n")
		for _, f := range c.LoadedEnvFiles {
			fmt.Fprintf(&b, "    - %s\n", f)
		}
	}
	return b.String()
}
