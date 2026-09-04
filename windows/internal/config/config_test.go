package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoundTripAtomic(t *testing.T) {
	dir := t.TempDir()
	store := &Store{Path: filepath.Join(dir, "config.json")}
	cfg := Default()
	cfg.Enabled = true
	cfg.DNS.DoHURL = "https://cloudflare-dns.com/dns-query"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.DNS.DoHURL == "" {
		t.Fatalf("%+v", got)
	}
}

func TestCorruptConfigFailsSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &Store{Path: path}
	_, err := store.Load()
	if err == nil {
		t.Fatal("expected corrupt error")
	}
}

func TestMissingConfigReturnsDefaults(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "missing.json")}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != SchemaVersion || cfg.Enabled {
		t.Fatalf("%+v", cfg)
	}
}
