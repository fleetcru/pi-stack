package server

import (
	"path/filepath"
	"testing"
)

func TestDeviceRegistryCreatesAuthenticatesAndRevokes(t *testing.T) {
	registry := newDeviceRegistry(filepath.Join(t.TempDir(), "devices.json"))
	record, token, err := registry.create("phone")
	if err != nil {
		t.Fatal(err)
	}
	if record.Name != "phone" || token == "" {
		t.Fatalf("unexpected device: %+v", record)
	}
	if id, ok := registry.authenticate(token); !ok || id != record.ID {
		t.Fatalf("device token did not authenticate: %q %v", id, ok)
	}
	if !registry.revoke(record.ID) {
		t.Fatal("device revoke failed")
	}
	if _, ok := registry.authenticate(token); ok {
		t.Fatal("revoked token authenticated")
	}
}

func TestDeviceRegistryRestoresHashedCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	first := newDeviceRegistry(path)
	record, token, err := first.create("laptop")
	if err != nil {
		t.Fatal(err)
	}
	second := newDeviceRegistry(path)
	if err := second.load(); err != nil {
		t.Fatal(err)
	}
	if id, ok := second.authenticate(token); !ok || id != record.ID {
		t.Fatalf("restored token did not authenticate: %q %v", id, ok)
	}
	for _, listed := range second.list() {
		if listed.TokenHash != "" {
			t.Fatal("device listing leaked token hash")
		}
	}
}
