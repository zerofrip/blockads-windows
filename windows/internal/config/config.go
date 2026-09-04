package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const SchemaVersion = 1

// Config is the versioned service configuration (PROGRAMDATA).
type Config struct {
	Version int            `json:"version"`
	Enabled bool           `json:"enabled"` // desired filtering-on at service start
	DNS     DNSConfig      `json:"dns"`
	Filters FiltersConfig  `json:"filters"`
}

type DNSConfig struct {
	ListenPort  int    `json:"listenPort"`
	Protocol    string `json:"protocol"` // udp, doh, …
	Primary     string `json:"primary"`
	Fallback    string `json:"fallback"`
	DoHURL      string `json:"dohUrl"`
}

type FiltersConfig struct {
	CatalogURL     string   `json:"catalogUrl"`
	EnabledListIDs []string `json:"enabledListIds"`
	AutoUpdate     bool     `json:"autoUpdate"`
}

const DefaultCatalogURL = "https://raw.githubusercontent.com/pass-with-high-score/blockads-default-filter/refs/heads/main/output/filter_lists.json"

func Default() Config {
	return Config{
		Version: SchemaVersion,
		Enabled: false,
		DNS: DNSConfig{
			ListenPort: 53,
			Protocol:   "udp",
			Primary:    "1.1.1.1",
			Fallback:   "1.0.0.1",
		},
		Filters: FiltersConfig{
			CatalogURL: DefaultCatalogURL,
			AutoUpdate: true,
		},
	}
}

func (c *Config) Validate() error {
	if c.Version != SchemaVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.DNS.ListenPort <= 0 || c.DNS.ListenPort > 65535 {
		return fmt.Errorf("invalid listenPort")
	}
	if c.DNS.Protocol == "" {
		c.DNS.Protocol = "udp"
	}
	if c.DNS.Primary == "" {
		return fmt.Errorf("primary DNS required")
	}
	if c.Filters.CatalogURL == "" {
		c.Filters.CatalogURL = DefaultCatalogURL
	}
	return nil
}

// Store persists config atomically.
type Store struct {
	Path string
}

func (s *Store) Load() (Config, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			return cfg, nil
		}
		return Config{}, err
	}
	// Windows PowerShell Set-Content -Encoding utf8 writes a BOM; strip it.
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("corrupt config (refusing destructive defaults apply): %w", err)
	}
	// Unknown fields are ignored by encoding/json (safe).
	if cfg.Version == 0 {
		return Config{}, errors.New("corrupt config: missing version")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (s *Store) Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

