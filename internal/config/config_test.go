package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	valid := Config{
		StatePath:       filepath.Join(t.TempDir(), "threadhall.db"),
		PublicURL:       "http://threadhall.test",
		WriterQueueSize: 1,
		ReadConnections: 1,
	}

	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "state path is required", change: func(c *Config) { c.StatePath = "" }},
		{name: "public URL is required", change: func(c *Config) { c.PublicURL = "" }},
		{name: "public URL must be absolute HTTP", change: func(c *Config) { c.PublicURL = "/threadhall" }},
		{name: "production requires secure cookies", change: func(c *Config) {
			c.Production = true
			c.PublicURL = "https://threadhall.test"
		}},
		{name: "production requires HTTPS", change: func(c *Config) {
			c.Production = true
			c.SecureCookies = true
		}},
		{name: "writer queue must be positive", change: func(c *Config) { c.WriterQueueSize = 0 }},
		{name: "writer queue is bounded", change: func(c *Config) { c.WriterQueueSize = maxWriterQueueSize + 1 }},
		{name: "read connections must be positive", change: func(c *Config) { c.ReadConnections = 0 }},
		{name: "read connections are bounded", change: func(c *Config) { c.ReadConnections = maxReadConnections + 1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.change(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	production := valid
	production.Production = true
	production.SecureCookies = true
	production.PublicURL = "https://threadhall.test"
	if err := production.Validate(); err != nil {
		t.Fatalf("valid production config: %v", err)
	}
}

func TestGenerateSecretFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.secret")
	if err := GenerateSecretFile(path); err != nil {
		t.Fatalf("GenerateSecretFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("secret mode = %#o, want 0600", got)
	}
	secret, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	if got := len(secret); got != secretBytes {
		t.Fatalf("secret length = %d, want %d", got, secretBytes)
	}
}
