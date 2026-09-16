package crypto

import (
	"encoding/hex"
	"strings"
	"testing"
)

// testKey is a deterministic 32-byte key for tests (64 hex chars).
const testKeyHex = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func testKey(t *testing.T) [32]byte {
	t.Helper()
	key, err := ParseMasterKey(testKeyHex)
	if err != nil {
		t.Fatalf("ParseMasterKey(%q): %v", testKeyHex, err)
	}
	return key
}

func TestRoundtrip(t *testing.T) {
	key := testKey(t)
	plaintext := "my-secret-token-123"

	env := Encrypt(key, plaintext)
	got, err := Decrypt(key, env)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("Decrypt returned %q, want %q", got, plaintext)
	}
}

func TestWrongKey(t *testing.T) {
	keyA := testKey(t)

	var keyB [32]byte
	keyB[0] = 0x01 // different from keyA

	plaintext := "sensitive-data"
	env := Encrypt(keyA, plaintext)

	_, err := Decrypt(keyB, env)
	if err == nil {
		t.Fatal("Decrypt with wrong key: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Decrypt with wrong key: expected 'authentication failed' error, got %q", err.Error())
	}
}

func TestV1PrefixParsing(t *testing.T) {
	key := testKey(t)
	plaintext := "prefix-test-data"

	env := Encrypt(key, plaintext)

	// Verify the envelope starts with v1: prefix.
	if !strings.HasPrefix(env, "v1:") {
		t.Fatalf("Envelope does not start with 'v1:': %q", env)
	}

	// Verify it has exactly 4 colon-delimited parts.
	parts := strings.Split(env, ":")
	if len(parts) != 4 {
		t.Fatalf("Envelope has %d parts, want 4", len(parts))
	}

	// Verify keyID is 8 hex chars.
	if len(parts[1]) != 8 {
		t.Fatalf("keyID length is %d, want 8", len(parts[1]))
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		t.Fatalf("keyID is not valid hex: %v", err)
	}

	// Verify roundtrip still works.
	got, err := Decrypt(key, env)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("Decrypt returned %q, want %q", got, plaintext)
	}
}

func TestUnknownVersion(t *testing.T) {
	key := testKey(t)

	// Craft an envelope with unsupported version.
	badEnv := "v2:abcdef01:abc123:def456"

	_, err := Decrypt(key, badEnv)
	if err == nil {
		t.Fatal("Decrypt with v2 envelope: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported envelope version: v2:") {
		t.Fatalf("Expected 'unsupported envelope version: v2:', got %q", err.Error())
	}
}

func TestNonceUniqueness(t *testing.T) {
	key := testKey(t)
	plaintext := "same-plaintext-every-time"

	env1 := Encrypt(key, plaintext)
	env2 := Encrypt(key, plaintext)

	if env1 == env2 {
		t.Fatal("Two encryptions of the same plaintext produced identical envelopes")
	}

	// Both should still decrypt correctly.
	got1, err := Decrypt(key, env1)
	if err != nil {
		t.Fatalf("Decrypt env1: %v", err)
	}
	got2, err := Decrypt(key, env2)
	if err != nil {
		t.Fatalf("Decrypt env2: %v", err)
	}
	if got1 != plaintext || got2 != plaintext {
		t.Fatalf("Roundtrip mismatch: got1=%q got2=%q want=%q", got1, got2, plaintext)
	}
}

func TestParseMasterKeyErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not hex", "zz"},
		{"wrong length", "abcd"},
		{"empty", ""},
		{"short hex", "abcdef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMasterKey(tt.input)
			if err == nil {
				t.Fatalf("ParseMasterKey(%q): expected error, got nil", tt.input)
			}
		})
	}
}

func TestMalformedEnvelope(t *testing.T) {
	key := testKey(t)

	tests := []struct {
		name     string
		envelope string
	}{
		{"too few parts", "v1:abc"},
		{"too many parts", "v1:a:b:c:d"},
		{"bad nonce base64", "v1:abcdef01:!!!:dGVzdA=="},
		{"bad ct base64", "v1:abcdef01:YWJjZGVmZw==:!!!"},
		{"empty envelope", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(key, tt.envelope)
			if err == nil {
				t.Fatalf("Decrypt with malformed envelope %q: expected error, got nil", tt.envelope)
			}
		})
	}
}