package filters

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tunnel "github.com/nqmgaming/blockads-tunnel"
)

const (
	trieMagic   = 0x54524945
	trieVersion = 2
	bloomMagic  = 0x424C4F4D
	bloomVersion = 1
	maxDownload = 64 << 20 // 64 MiB
)

// CatalogEntry mirrors Android remote filter_lists.json entries.
type CatalogEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsEnabled   bool   `json:"isEnabled"`
	Category    string `json:"category"`
	RuleCount   int    `json:"ruleCount"`
	BloomURL    string `json:"bloomUrl"`
	TrieURL     string `json:"trieUrl"`
	CSSURL      string `json:"cssUrl"`
	Scriptlets  string `json:"scriptletsUrl"`
}

// Manager owns filter download and activation under PROGRAMDATA.
type Manager struct {
	Dir    string
	Client *http.Client
}

func NewManager(dir string) *Manager {
	return &Manager{
		Dir: dir,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (m *Manager) catalogPath() string { return filepath.Join(m.Dir, "catalog.json") }

func (m *Manager) FetchCatalog(ctx context.Context, url string) ([]CatalogEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("catalog HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var entries []CatalogEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(m.Dir, 0o755)
	tmp := m.catalogPath() + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, m.catalogPath()); err != nil {
		return nil, err
	}
	return entries, nil
}

type LoadedPaths struct {
	AdTrieCSV   string
	AdBloomCSV  string
	SecTrieCSV  string
	SecBloomCSV string
	ListIDs     []string
}

// DownloadAndStage downloads enabled lists into staging and validates magic.
func (m *Manager) DownloadAndStage(ctx context.Context, entries []CatalogEntry, enabledIDs []string) (LoadedPaths, error) {
	want := map[string]bool{}
	if len(enabledIDs) == 0 {
		for _, e := range entries {
			if e.IsEnabled {
				want[e.ID] = true
			}
		}
	} else {
		for _, id := range enabledIDs {
			want[id] = true
		}
	}

	var paths LoadedPaths
	for _, e := range entries {
		if !want[e.ID] {
			continue
		}
		listDir := filepath.Join(m.Dir, "staging", e.ID)
		_ = os.MkdirAll(listDir, 0o755)
		triePath := filepath.Join(listDir, "current.trie")
		bloomPath := filepath.Join(listDir, "current.bloom")
		if err := m.downloadFile(ctx, e.TrieURL, triePath); err != nil {
			return LoadedPaths{}, fmt.Errorf("%s trie: %w", e.ID, err)
		}
		if err := m.downloadFile(ctx, e.BloomURL, bloomPath); err != nil {
			return LoadedPaths{}, fmt.Errorf("%s bloom: %w", e.ID, err)
		}
		if err := validateTrieFile(triePath); err != nil {
			return LoadedPaths{}, fmt.Errorf("%s trie validate: %w", e.ID, err)
		}
		if err := validateBloomFile(bloomPath); err != nil {
			return LoadedPaths{}, fmt.Errorf("%s bloom validate: %w", e.ID, err)
		}
		// Prove mmap load
		tr, err := tunnel.LoadMmapTrie(triePath)
		if err != nil {
			return LoadedPaths{}, err
		}
		tr.Close()
		bl, err := tunnel.LoadBloomFilter(bloomPath)
		if err != nil {
			return LoadedPaths{}, err
		}
		bl.Close()

		cat := "AD"
		if e.Category == "SECURITY" {
			cat = "SECURITY"
		}
		if cat == "SECURITY" {
			paths.SecTrieCSV = joinCSV(paths.SecTrieCSV, triePath)
			paths.SecBloomCSV = joinCSV(paths.SecBloomCSV, bloomPath)
		} else {
			paths.AdTrieCSV = joinCSV(paths.AdTrieCSV, triePath)
			paths.AdBloomCSV = joinCSV(paths.AdBloomCSV, bloomPath)
		}
		paths.ListIDs = append(paths.ListIDs, e.ID)
	}
	return paths, nil
}

// ActivateStaging moves staging → lists/<id> after engine accepted the paths.
func (m *Manager) ActivateStaging(listIDs []string) error {
	for _, id := range listIDs {
		src := filepath.Join(m.Dir, "staging", id)
		dst := filepath.Join(m.Dir, "lists", id)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.RemoveAll(dst)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// PathsFromCurrent returns CSV paths from activated lists/.
func (m *Manager) PathsFromCurrent(entries []CatalogEntry, enabledIDs []string) (LoadedPaths, error) {
	want := map[string]bool{}
	for _, id := range enabledIDs {
		want[id] = true
	}
	if len(want) == 0 {
		for _, e := range entries {
			if e.IsEnabled {
				want[e.ID] = true
			}
		}
	}
	var paths LoadedPaths
	for _, e := range entries {
		if !want[e.ID] {
			continue
		}
		triePath := filepath.Join(m.Dir, "lists", e.ID, "current.trie")
		bloomPath := filepath.Join(m.Dir, "lists", e.ID, "current.bloom")
		if _, err := os.Stat(triePath); err != nil {
			continue
		}
		if e.Category == "SECURITY" {
			paths.SecTrieCSV = joinCSV(paths.SecTrieCSV, triePath)
			paths.SecBloomCSV = joinCSV(paths.SecBloomCSV, bloomPath)
		} else {
			paths.AdTrieCSV = joinCSV(paths.AdTrieCSV, triePath)
			paths.AdBloomCSV = joinCSV(paths.AdBloomCSV, bloomPath)
		}
		paths.ListIDs = append(paths.ListIDs, e.ID)
	}
	return paths, nil
}

func joinCSV(existing, path string) string {
	if existing == "" {
		return path
	}
	return existing + "," + path
}

func (m *Manager) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		f.Close()
		return err
	}
	if n > maxDownload {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("download exceeds size limit")
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func validateTrieFile(path string) error {
	b, err := readHeader(path, 16)
	if err != nil {
		return err
	}
	if binary.BigEndian.Uint32(b[0:4]) != trieMagic {
		return fmt.Errorf("bad trie magic")
	}
	if binary.BigEndian.Uint32(b[4:8]) != trieVersion {
		return fmt.Errorf("bad trie version")
	}
	return nil
}

func validateBloomFile(path string) error {
	b, err := readHeader(path, 24)
	if err != nil {
		return err
	}
	if binary.BigEndian.Uint32(b[0:4]) != bloomMagic {
		return fmt.Errorf("bad bloom magic")
	}
	if binary.BigEndian.Uint32(b[4:8]) != bloomVersion {
		return fmt.Errorf("bad bloom version")
	}
	return nil
}

func readHeader(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
