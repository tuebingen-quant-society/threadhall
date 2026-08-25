package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMemory        uint32 = 64 * 1024
	passwordTime          uint32 = 3
	passwordThreads       uint8  = 1
	passwordSaltBytes            = 16
	passwordKeyBytes             = 32
	maxPasswordHashLength        = 160
)

var passwordEncoding = base64.RawStdEncoding.Strict()

// HashPassword applies Threadhall's fixed Argon2id policy and encodes it in PHC form.
func HashPassword(password string, random io.Reader) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	if random == nil {
		return "", fmt.Errorf("password salt source is required")
	}
	var salt [passwordSaltBytes]byte
	if _, err := io.ReadFull(random, salt[:]); err != nil {
		return "", fmt.Errorf("read password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt[:], passwordTime, passwordMemory, passwordThreads, passwordKeyBytes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, passwordMemory, passwordTime, passwordThreads,
		passwordEncoding.EncodeToString(salt[:]), passwordEncoding.EncodeToString(key)), nil
}

// VerifyPassword rejects every non-canonical or out-of-policy hash before
// invoking Argon2, preventing attacker-selected memory or parallelism costs.
func VerifyPassword(password, encoded string) bool {
	if validatePassword(password) != nil || len(encoded) > maxPasswordHashLength {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" ||
		parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=1" ||
		len(parts[4]) != passwordEncoding.EncodedLen(passwordSaltBytes) ||
		len(parts[5]) != passwordEncoding.EncodedLen(passwordKeyBytes) {
		return false
	}
	var salt [passwordSaltBytes]byte
	if written, err := passwordEncoding.Decode(salt[:], []byte(parts[4])); err != nil || written != len(salt) {
		return false
	}
	var expected [passwordKeyBytes]byte
	if written, err := passwordEncoding.Decode(expected[:], []byte(parts[5])); err != nil || written != len(expected) {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt[:], passwordTime, passwordMemory, passwordThreads, passwordKeyBytes)
	return subtle.ConstantTimeCompare(actual, expected[:]) == 1
}
