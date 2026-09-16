// Package crypto provides NaCl secretbox-based envelope encryption for
// credential storage. The envelope format is versioned to support key
// rotation:
//
//	v1:{keyID}:{base64(nonce)}:{base64(ciphertext)}
//
// keyID is the first 8 hex characters of SHA256(key) — for identification
// only, NOT secret. The nonce is 24 random bytes generated per encryption.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/nacl/secretbox"
)

const (
	// envelopeVersion is the only supported envelope format version.
	envelopeVersion = "v1"

	// nonceLen is the NaCl secretbox nonce length (24 bytes).
	nonceLen = 24

	// keyHexLen is the expected length of a hex-encoded 32-byte key.
	keyHexLen = 64
)

// ParseMasterKey decodes a 64-character hex string into a [32]byte key.
// Returns an error if the length is wrong or the string is not valid hex.
func ParseMasterKey(hexStr string) ([32]byte, error) {
	var key [32]byte

	if len(hexStr) != keyHexLen {
		return key, fmt.Errorf(
			"master key must be exactly %d hex characters, got %d",
			keyHexLen, len(hexStr),
		)
	}

	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		return key, fmt.Errorf("invalid hex in master key: %w", err)
	}

	copy(key[:], raw)
	return key, nil
}

// Encrypt encrypts plaintext using NaCl secretbox and returns a versioned
// envelope string. Each call generates a fresh random nonce, so the same
// plaintext produces different ciphertexts every time.
func Encrypt(key [32]byte, plaintext string) string {
	// keyID = first 8 hex chars of SHA256(key) — identification only.
	h := sha256.Sum256(key[:])
	keyID := hex.EncodeToString(h[:4])

	// Generate a fresh 24-byte nonce.
	var nonce [nonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		// crypto/rand should never fail in practice.
		panic("crypto/rand.Read: " + err.Error())
	}

	// Encrypt with secretbox. Seal appends the encrypted message to out.
	ct := secretbox.Seal(nil, []byte(plaintext), &nonce, &key)

	return fmt.Sprintf("%s:%s:%s:%s",
		envelopeVersion,
		keyID,
		base64.RawStdEncoding.EncodeToString(nonce[:]),
		base64.RawStdEncoding.EncodeToString(ct),
	)
}

// Decrypt parses a versioned envelope string and decrypts the ciphertext
// using the provided key. Returns an error if the version is unsupported,
// the envelope is malformed, or authentication fails (wrong key / tampered
// ciphertext).
func Decrypt(key [32]byte, envelope string) (string, error) {
	parts := strings.SplitN(envelope, ":", 4)
	if len(parts) != 4 {
		return "", fmt.Errorf(
			"malformed envelope: expected 4 colon-delimited parts, got %d",
			len(parts),
		)
	}

	version := parts[0]
	if version != envelopeVersion {
		return "", fmt.Errorf("unsupported envelope version: %s:", version)
	}

	// parts[1] is keyID — not validated here (identification only).

	nonceBytes, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid nonce base64: %w", err)
	}
	if len(nonceBytes) != nonceLen {
		return "", fmt.Errorf(
			"invalid nonce length: got %d bytes, want %d",
			len(nonceBytes), nonceLen,
		)
	}

	ct, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", fmt.Errorf("invalid ciphertext base64: %w", err)
	}

	var nonce [nonceLen]byte
	copy(nonce[:], nonceBytes)

	result, ok := secretbox.Open(nil, ct, &nonce, &key)
	if !ok {
		return "", fmt.Errorf("decryption failed: ciphertext authentication failed")
	}

	return string(result), nil
}