// Package config validates Threadhall's required runtime configuration.
package config

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	maxWriterQueueSize = 4096
	maxReadConnections = 16
	secretBytes        = 32
)

// Config contains the explicit storage and browser-security settings needed at
// startup. It deliberately supplies no defaults.
type Config struct {
	StatePath       string
	PublicURL       string
	Production      bool
	SecureCookies   bool
	WriterQueueSize int
	ReadConnections int
}

// Validate rejects incomplete, unsafe, or unbounded configuration.
func (c Config) Validate() error {
	if strings.TrimSpace(c.StatePath) == "" {
		return errors.New("state path is required")
	}
	publicURL, err := url.Parse(c.PublicURL)
	if err != nil || publicURL.Host == "" ||
		(publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		return errors.New("public URL must be an absolute HTTP or HTTPS URL")
	}
	publicOrigin := publicURL.Scheme + "://" + publicURL.Host
	if c.PublicURL != publicOrigin || publicURL.User != nil ||
		publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return errors.New("public URL must be an exact origin without path, credentials, query, or fragment")
	}
	if c.Production && !c.SecureCookies {
		return errors.New("production requires secure cookies")
	}
	if c.Production && publicURL.Scheme != "https" {
		return errors.New("production requires an HTTPS public URL")
	}
	if c.WriterQueueSize <= 0 || c.WriterQueueSize > maxWriterQueueSize {
		return fmt.Errorf("writer queue size must be between 1 and %d", maxWriterQueueSize)
	}
	if c.ReadConnections <= 0 || c.ReadConnections > maxReadConnections {
		return fmt.Errorf("read connections must be between 1 and %d", maxReadConnections)
	}
	return nil
}

// GenerateSecretFile creates a new random secret readable only by its owner.
// It refuses to replace an existing file. A failed write, sync, or close may
// leave that owner-only file in place; it is never removed by pathname.
func GenerateSecretFile(path string) (err error) {
	secret := make([]byte, secretBytes)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate secret: %w", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create secret file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close secret file: %w", closeErr)
		}
	}()

	if _, err = file.Write(secret); err != nil {
		return fmt.Errorf("write secret file: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync secret file: %w", err)
	}
	return nil
}
