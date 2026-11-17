package testutil

import (
	"crypto/rand"
	"testing"
)

func GuardrailAWSKey(t testing.TB) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("rand: %v", err)
	}
	alphabet := []byte("ABCDEFGHJKLMNPQRSTUVWXYZ234567") // pragma: allowlist secret
	out := make([]byte, len(raw))
	for i, b := range raw {
		j := int(b) % len(alphabet)
		out[i] = alphabet[j]
		alphabet = append(alphabet[:j], alphabet[j+1:]...)
	}
	return "AKIA" + string(out)
}
