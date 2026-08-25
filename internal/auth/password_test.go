package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestHashPasswordEncodesRequiredArgon2idPolicy(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashPassword(password, bytes.NewReader(bytes.Repeat([]byte{0x2a}, 16)))
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	const prefix = "$argon2id$v=19$m=65536,t=3,p=1$"
	if !strings.HasPrefix(hash, prefix) {
		t.Fatalf("hash = %q, want prefix %q", hash, prefix)
	}
	if ok := VerifyPassword(password, hash); !ok {
		t.Fatal("VerifyPassword rejected the password used to create the hash")
	}
	if ok := VerifyPassword("wrong password", hash); ok {
		t.Fatal("VerifyPassword accepted a different password")
	}
}

func TestVerifyPasswordRejectsMalformedOrOutOfPolicyHashes(t *testing.T) {
	valid, err := HashPassword("correct horse battery staple", bytes.NewReader(bytes.Repeat([]byte{7}, 16)))
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	tests := []string{
		"",
		"$argon2i$v=19$m=65536,t=3,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		strings.Replace(valid, "v=19", "v=20", 1),
		strings.Replace(valid, "m=65536", "m=1048576", 1),
		strings.Replace(valid, "t=3", "t=4", 1),
		strings.Replace(valid, "p=1", "p=2", 1),
		valid + "A",
		strings.Repeat("x", 512),
	}
	for _, encoded := range tests {
		if VerifyPassword("correct horse battery staple", encoded) {
			t.Errorf("VerifyPassword accepted malformed or out-of-policy hash %q", encoded)
		}
	}
}

func TestHashPasswordRequiresExactlyOneSaltRead(t *testing.T) {
	if _, err := HashPassword("correct horse battery staple", bytes.NewReader(make([]byte, 15))); err == nil {
		t.Fatal("HashPassword error = nil, want short-randomness error")
	}
}
