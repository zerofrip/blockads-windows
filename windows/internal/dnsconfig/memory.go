package dnsconfig

import (
	"fmt"
	"sync"
)

// MemoryConfigurator is an in-memory DnsConfigurator for unit tests.
type MemoryConfigurator struct {
	mu       sync.Mutex
	Adapters []NetworkAdapter
	DNS      map[string]AdapterDNSSnapshot // guid → snapshot
	Fails    map[string]error             // guid → inject error on Apply/Restore
}

func NewMemoryConfigurator(adapters []NetworkAdapter, initial map[string]AdapterDNSSnapshot) *MemoryConfigurator {
	m := &MemoryConfigurator{
		Adapters: append([]NetworkAdapter(nil), adapters...),
		DNS:      make(map[string]AdapterDNSSnapshot),
		Fails:    make(map[string]error),
	}
	for k, v := range initial {
		cp := v
		FillChecksum(&cp)
		m.DNS[k] = cp
	}
	return m
}

func (m *MemoryConfigurator) ListAdapters() ([]NetworkAdapter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]NetworkAdapter(nil), m.Adapters...), nil
}

func (m *MemoryConfigurator) Snapshot(key AdapterKey) (AdapterDNSSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.DNS[key.GUID]
	if !ok {
		return AdapterDNSSnapshot{}, fmt.Errorf("unknown adapter %s", key.GUID)
	}
	s.Key = key
	return s, nil
}

func (m *MemoryConfigurator) ApplyLocalhost(key AdapterKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.Fails[key.GUID]; ok && err != nil {
		return err
	}
	s := m.DNS[key.GUID]
	s.Key = key
	s.IPv4Servers = append(DNSServerList(nil), LocalhostApplied.IPv4Servers...)
	s.IPv6Servers = append(DNSServerList(nil), LocalhostApplied.IPv6Servers...)
	s.IPv4DHCP = false
	s.IPv6DHCP = false
	FillChecksum(&s)
	m.DNS[key.GUID] = s
	return nil
}

func (m *MemoryConfigurator) Restore(key AdapterKey, original AdapterDNSSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.Fails[key.GUID]; ok && err != nil {
		return err
	}
	cp := original
	cp.Key = key
	FillChecksum(&cp)
	m.DNS[key.GUID] = cp
	return nil
}

func (m *MemoryConfigurator) Status() (string, error) {
	return fmt.Sprintf("memory adapters=%d", len(m.Adapters)), nil
}

// SetCurrent replaces DNS for tests (simulates external change).
func (m *MemoryConfigurator) SetCurrent(key AdapterKey, snap AdapterDNSSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap.Key = key
	FillChecksum(&snap)
	m.DNS[key.GUID] = snap
}
