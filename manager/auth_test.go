package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthProviderAcceptsGeneratedPassword(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "web.yml")
	hash, err := hashPassword([]byte("secret123"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("basic_auth_users:\n  admin: "+hash+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := &authProvider{path: path}

	valid := httptest.NewRequest("GET", "http://example.test/", nil)
	valid.SetBasicAuth("admin", "secret123")
	if !provider.authenticate(valid) {
		t.Fatal("valid credentials were rejected")
	}

	invalid := httptest.NewRequest("GET", "http://example.test/", nil)
	invalid.SetBasicAuth("admin", "wrong")
	if provider.authenticate(invalid) {
		t.Fatal("invalid credentials were accepted")
	}
}

func TestAuthProviderAllowsEmptyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.yml")
	if err := os.WriteFile(path, []byte("\n"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "http://example.test/", nil)
	if !(&authProvider{path: path}).authenticate(request) {
		t.Fatal("empty auth config should disable authentication")
	}
}
