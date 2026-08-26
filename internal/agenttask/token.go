package agenttask

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const workerTokenBytes = 32

func ValidUsername(username string) bool {
	if username == "" || len(username) > 64 || !utf8.ValidString(username) || strings.TrimSpace(username) != username {
		return false
	}
	for index, character := range username {
		if unicode.IsLetter(character) || unicode.IsDigit(character) ||
			(index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return true
}

// NewWorkerToken returns the only raw copy of a worker bearer and its storage hash.
func NewWorkerToken(random io.Reader) (string, [32]byte, error) {
	var raw [workerTokenBytes]byte
	if _, err := io.ReadFull(random, raw[:]); err != nil {
		return "", [32]byte{}, fmt.Errorf("read worker token randomness: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), sha256.Sum256(raw[:]), nil
}
