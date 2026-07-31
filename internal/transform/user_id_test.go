package transform

import (
	"strings"
	"testing"
)

func authBearer() string {
	return "Bearer sk-test-key-12345"
}

func authDifferent() string {
	return "Bearer sk-other-key-67890"
}

func authEmpty() string {
	return ""
}

func authNoPrefix() string {
	return "sk-test-key-noprefix"
}

// --- NormalizeUpstreamUserID valid explicit user_id ---

func TestUserIDExplicitValidPreserved(t *testing.T) {
	payload := map[string]any{"user_id": "customer_123"}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "customer_123" {
		t.Errorf("got %q, want customer_123", uid)
	}
}

func TestUserIDExplicitWithUnderscoreAndDash(t *testing.T) {
	payload := map[string]any{"user_id": "user_name-123"}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "user_name-123" {
		t.Errorf("got %q", uid)
	}
}

func TestUserIDMaxLength(t *testing.T) {
	longID := strings.Repeat("a", 512)
	payload := map[string]any{"user_id": longID}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != longID {
		t.Errorf("512-char user_id should be accepted")
	}
}

// --- NormalizeUpstreamUserID invalid explicit user_id ---

func TestUserIDInvalidCharRejected(t *testing.T) {
	payload := map[string]any{"user_id": "github|63306485"}
	_, err := NormalizeUpstreamUserID(payload, authBearer())
	if err == nil {
		t.Fatal("expected error for invalid chars")
	}
	ve, ok := err.(*RequestValidationError)
	if !ok {
		t.Fatalf("expected *RequestValidationError, got %T", err)
	}
	if ve.Param != "user_id" {
		t.Errorf("param: got %q, want user_id", ve.Param)
	}
}

func TestUserIDTooLongRejected(t *testing.T) {
	longID := strings.Repeat("a", 513)
	payload := map[string]any{"user_id": longID}
	_, err := NormalizeUpstreamUserID(payload, authBearer())
	if err == nil {
		t.Fatal("expected error for >512 chars")
	}
}

func TestUserIDNonStringRejected(t *testing.T) {
	payload := map[string]any{"user_id": 123}
	_, err := NormalizeUpstreamUserID(payload, authBearer())
	if err == nil {
		t.Fatal("expected error for non-string user_id")
	}
}

func TestUserIDNullOmitted(t *testing.T) {
	payload := map[string]any{"user_id": nil}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "" {
		t.Errorf("got %q, want empty", uid)
	}
}

func TestUserIDEmptyOmitted(t *testing.T) {
	payload := map[string]any{"user_id": ""}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "" {
		t.Errorf("got %q, want empty", uid)
	}
}

func TestUserIDWhitespaceOmitted(t *testing.T) {
	payload := map[string]any{"user_id": "   "}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "" {
		t.Errorf("got %q, want empty", uid)
	}
}

// --- OpenAI user → generated user_id ---

func TestOpenAIUserMapsToGenerated(t *testing.T) {
	payload := map[string]any{"user": "github|63306485"}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(uid, "u_") {
		t.Errorf("generated ID must start with u_: %q", uid)
	}
	if len(uid) != 34 {
		t.Errorf("generated ID length: got %d, want 34", len(uid))
	}
	// Must match ^u_[a-f0-9]{32}$
	hexPart := uid[2:]
	if len(hexPart) != 32 {
		t.Errorf("hex part length: got %d, want 32", len(hexPart))
	}
	for _, c := range hexPart {
		if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
			t.Errorf("generated ID contains invalid hex char: %c in %q", c, uid)
			break
		}
	}
}

func TestOpenAIUserDeterministic(t *testing.T) {
	payload := map[string]any{"user": "github|63306485"}
	a, _ := NormalizeUpstreamUserID(payload, authBearer())
	b, _ := NormalizeUpstreamUserID(payload, authBearer())
	if a != b {
		t.Errorf("same user + same auth: got %q vs %q", a, b)
	}
}

func TestOpenAIUserDifferentUserChangesID(t *testing.T) {
	a, _ := NormalizeUpstreamUserID(map[string]any{"user": "github|63306485"}, authBearer())
	b, _ := NormalizeUpstreamUserID(map[string]any{"user": "github|99999999"}, authBearer())
	if a == b {
		t.Error("different user must produce different ID")
	}
}

func TestOpenAIUserDifferentAuthChangesID(t *testing.T) {
	payload := map[string]any{"user": "github|63306485"}
	a, _ := NormalizeUpstreamUserID(payload, authBearer())
	b, _ := NormalizeUpstreamUserID(payload, authDifferent())
	if a == b {
		t.Error("different API key must produce different ID")
	}
}

func TestOpenAIUserNullOmitted(t *testing.T) {
	payload := map[string]any{"user": nil}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "" {
		t.Errorf("got %q, want empty", uid)
	}
}

func TestOpenAIUserEmptyOmitted(t *testing.T) {
	payload := map[string]any{"user": ""}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "" {
		t.Errorf("got %q, want empty", uid)
	}
}

func TestOpenAIUserWhitespaceOmitted(t *testing.T) {
	payload := map[string]any{"user": "   "}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "" {
		t.Errorf("got %q, want empty", uid)
	}
}

func TestNonStringUserRejected(t *testing.T) {
	payload := map[string]any{"user": 42}
	_, err := NormalizeUpstreamUserID(payload, authBearer())
	if err == nil {
		t.Fatal("expected error for non-string user")
	}
	ve, ok := err.(*RequestValidationError)
	if !ok {
		t.Fatalf("expected *RequestValidationError, got %T", err)
	}
	if ve.Param != "user" {
		t.Errorf("param: got %q, want user", ve.Param)
	}
}

// --- Both fields: explicit user_id wins ---

func TestBothFieldsExplicitWins(t *testing.T) {
	payload := map[string]any{
		"user":    "github|63306485",
		"user_id": "customer_123",
	}
	uid, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "customer_123" {
		t.Errorf("got %q, want customer_123", uid)
	}
}

// --- No identity ---

func TestNoIdentity(t *testing.T) {
	uid, err := NormalizeUpstreamUserID(map[string]any{}, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "" {
		t.Errorf("got %q, want empty", uid)
	}
}

// --- Payload not mutated ---

func TestPayloadNotMutated(t *testing.T) {
	payload := map[string]any{
		"user":     "github|63306485",
		"user_id":  "explicit",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	origUser := payload["user"]
	origUserID := payload["user_id"]
	_, err := NormalizeUpstreamUserID(payload, authBearer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload["user"] != origUser {
		t.Error("payload mutated: user")
	}
	if payload["user_id"] != origUserID {
		t.Error("payload mutated: user_id")
	}
}

// --- Generated ID never contains raw user or token ---

func TestGeneratedIDNoRawUser(t *testing.T) {
	uid, _ := NormalizeUpstreamUserID(map[string]any{"user": "github|63306485"}, authBearer())
	if strings.Contains(uid, "63306485") {
		t.Errorf("generated ID must not contain raw user: %q", uid)
	}
	if strings.Contains(uid, "github") {
		t.Errorf("generated ID must not contain raw user: %q", uid)
	}
}

func TestGeneratedIDNoBearerToken(t *testing.T) {
	uid, _ := NormalizeUpstreamUserID(map[string]any{"user": "testuser"}, authBearer())
	if strings.Contains(uid, "sk-test-key") {
		t.Errorf("generated ID must not contain bearer token: %q", uid)
	}
}

// --- Empty authorization ---

func TestEmptyAuthorizationProducesID(t *testing.T) {
	uid, err := NormalizeUpstreamUserID(map[string]any{"user": "testuser"}, authEmpty())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid == "" {
		t.Error("should produce a fallback ID even with empty auth")
	}
	if strings.HasPrefix(uid, "u_") != true {
		t.Errorf("expected u_ prefix: %q", uid)
	}
}

func TestNoBearerPrefixAuthorization(t *testing.T) {
	payload := map[string]any{"user": "testuser"}
	a, _ := NormalizeUpstreamUserID(payload, authNoPrefix())
	b, _ := NormalizeUpstreamUserID(payload, authBearer())
	if a == b {
		t.Error("different key format should produce different IDs")
	}
}

// --- Spaces in user_id ---

func TestUserIDWithSpacesRejected(t *testing.T) {
	payload := map[string]any{"user_id": "contains spaces"}
	_, err := NormalizeUpstreamUserID(payload, authBearer())
	if err == nil {
		t.Fatal("expected error for spaces in user_id")
	}
}

func TestUserIDWithAtRejected(t *testing.T) {
	payload := map[string]any{"user_id": "email@example.com"}
	_, err := NormalizeUpstreamUserID(payload, authBearer())
	if err == nil {
		t.Fatal("expected error for @ in user_id")
	}
}
