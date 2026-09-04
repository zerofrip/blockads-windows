package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

func safetyPaths(t *testing.T) Paths {
	t.Helper()
	dir := t.TempDir()
	return Paths{
		DataDir:    dir,
		StateFile:  filepath.Join(dir, "recovery.json"),
		ConfigFile: filepath.Join(dir, "config.json"),
		FilterDir:  filepath.Join(dir, "filters"),
	}
}

func writePortCfg(t *testing.T, path string, port int, enabled bool) {
	t.Helper()
	content := fmt.Sprintf(`{
  "version": 1,
  "enabled": %t,
  "dns": {
    "listenPort": %d,
    "protocol": "udp",
    "primary": "1.1.1.1",
    "fallback": "1.0.0.1"
  },
  "filters": {
    "catalogUrl": "http://127.0.0.1:1/missing.json",
    "enabledListIds": [],
    "autoUpdate": false
  }
}`, enabled, port)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func ethAdapter(key dnsconfig.AdapterKey) dnsconfig.NetworkAdapter {
	return dnsconfig.NetworkAdapter{
		Key: key, FriendlyName: "Ethernet", Description: "Realtek PCIe",
		IfType: dnsconfig.IfTypeEthernetCSMACD, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"192.168.1.10"},
	}
}

func dhcpOrig(key dnsconfig.AdapterKey) dnsconfig.AdapterDNSSnapshot {
	s := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4DHCP: true, IPv6DHCP: true}
	dnsconfig.FillChecksum(&s)
	return s
}

// TestNetworkFlapDoesNotPoisonOriginalDns reproduces the 2026-09-04 incident sequence.
func TestNetworkFlapDoesNotPoisonOriginalDns(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{INCIDENT-BBBB-CCCC-DDDD-EEEEEEEEEEEE}", LUID: 42}
	orig := dhcpOrig(key)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})
	paths := safetyPaths(t)
	writePortCfg(t, paths.ConfigFile, 1960, false)
	c, err := New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Enable(ctx); err != nil {
		t.Fatalf("enable: %v", err)
	}
	st, _ := c.cfgStore.Load()
	if st == nil || len(st.Adapters) != 1 {
		t.Fatalf("ownership missing: %+v", st)
	}
	if !st.Adapters[0].Original.IPv4DHCP {
		t.Fatalf("original must be DHCP, got %+v", st.Adapters[0].Original)
	}
	savedOrig := st.Adapters[0].Original

	mem.RemoveAdapter(key.GUID)
	if err := c.ReevaluateAdapters(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = c.cfgStore.Load()
	if len(st.Adapters) != 1 {
		t.Fatalf("DNS-2: ownership dropped on flap: %+v", st)
	}
	if st.Adapters[0].Original.Checksum != savedOrig.Checksum {
		t.Fatalf("Original mutated on disappear")
	}

	local := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	dnsconfig.FillChecksum(&local)
	a := ethAdapter(key)
	a.Key.LUID = 99 // interface index/LUID churn must not re-key ownership
	a.FriendlyName = "Ethernet 2"
	mem.AddAdapter(a, local)
	if err := c.ReevaluateAdapters(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = c.cfgStore.Load()
	if st.Adapters[0].Original.Checksum != savedOrig.Checksum {
		t.Fatalf("Original poisoned to localhost/new snap: %+v", st.Adapters[0].Original)
	}
	if st.Adapters[0].Phase != dnsconfig.OwnershipOwned {
		t.Fatalf("phase=%s", st.Adapters[0].Phase)
	}

	if err := c.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	cur, _ := mem.Snapshot(key)
	if dnsconfig.LooksLikeLocalhostDNS(cur) {
		t.Fatalf("localhost remained after disable: %+v", cur)
	}
	if !cur.IPv4DHCP && len(cur.IPv4Servers) != 0 {
		t.Fatalf("expected DHCP restore, got %+v", cur)
	}
	st, _ = c.cfgStore.Load()
	if st != nil && len(st.Adapters) > 0 {
		t.Fatalf("ownership must be cleared after verified restore: %+v", st)
	}
	status := c.Status()
	if status.DNS.RecoveryRequired {
		t.Fatalf("recoveryRequired should be false: %+v", status.DNS)
	}
	if status.Engine.FilteringEnabled {
		t.Fatal("listener/filtering should be stopped")
	}
}

func TestNetworkFlapExternalDnsChangeIsPreserved(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{FLAPEXT-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"1.1.1.1"}}
	dnsconfig.FillChecksum(&orig)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})
	paths := safetyPaths(t)
	writePortCfg(t, paths.ConfigFile, 1961, false)
	c, err := New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	mem.RemoveAdapter(key.GUID)
	_ = c.ReevaluateAdapters(ctx)

	ext := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"8.8.4.4"}}
	dnsconfig.FillChecksum(&ext)
	mem.AddAdapter(ethAdapter(key), ext)
	// Ownership still claims Applied=localhost but current is external — Disable must not overwrite.
	if err := c.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	cur, _ := mem.Snapshot(key)
	if !dnsconfig.EqualServers(cur.IPv4Servers, ext.IPv4Servers) {
		t.Fatalf("external DNS overwritten: %v", cur.IPv4Servers)
	}
}

func TestStartupUnprovenLocalhostNotMutated(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{UNPROVEN-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orphan := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	dnsconfig.FillChecksum(&orphan)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orphan})
	paths := safetyPaths(t)
	writePortCfg(t, paths.ConfigFile, 1962, false)
	c, err := New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	beforeCalls := mem.RestoreCalls
	if err := c.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mem.RestoreCalls != beforeCalls {
		t.Fatalf("UNPROVEN_LOCALHOST must not be auto-mutated")
	}
	cur, _ := mem.Snapshot(key)
	if !dnsconfig.LooksLikeLocalhostDNS(cur) {
		t.Fatal("localhost should remain for unproven")
	}
	st := c.Status()
	if !st.DNS.RecoveryRequired {
		t.Fatal("expected RECOVERY_REQUIRED diagnostic")
	}
	if len(st.DNS.SuspectedUnprovenLocalhost) == 0 {
		t.Fatal("expected suspected unproven GUID listed")
	}
}

func TestStartupProvenOwnershipRestores(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{PROVEN--BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dhcpOrig(key)
	local := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	dnsconfig.FillChecksum(&local)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: local})
	paths := safetyPaths(t)
	writePortCfg(t, paths.ConfigFile, 1963, false)
	store := dnsconfig.NewStateStore(paths.StateFile)
	own, err := dnsconfig.BeginOwnership(key, orig, "sess", 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = own.MarkOwned()
	if err := store.Save(&dnsconfig.RecoveryState{
		Version: dnsconfig.RecoveryStateVersion, PolicyVersion: dnsconfig.AdapterPolicyVersion,
		SessionID: "sess", Controller: dnsconfig.StateRecoveryRequired,
		Adapters: []dnsconfig.AdapterOwnership{own}, Dirty: true, SavedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	c, err := New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	cur, _ := mem.Snapshot(key)
	if dnsconfig.LooksLikeLocalhostDNS(cur) {
		t.Fatalf("proven ownership should restore: %+v", cur)
	}
}

func TestEmergencyRestoreSkipsUnprovenByDefault(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{EMERG---BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orphan := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	dnsconfig.FillChecksum(&orphan)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orphan})
	paths := safetyPaths(t)
	res, err := EmergencyRestore(paths, mem, EmergencyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plan) != 1 || res.Plan[0].Action != "skip_unproven" {
		t.Fatalf("plan=%+v", res.Plan)
	}
	cur, _ := mem.Snapshot(key)
	if !dnsconfig.LooksLikeLocalhostDNS(cur) {
		t.Fatal("default emergency-restore must not mutate unproven")
	}
	res, err = EmergencyRestore(paths, mem, EmergencyOptions{ForceUnprovenLocalhost: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plan) != 1 || res.Plan[0].Action != "force_unproven" {
		t.Fatalf("force plan=%+v", res.Plan)
	}
	cur, _ = mem.Snapshot(key)
	if dnsconfig.LooksLikeLocalhostDNS(cur) {
		t.Fatal("force should clear unproven localhost")
	}
}

func TestWatchdogRestoreSuccessDegradedNoOwnership(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{WDGOK---BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dhcpOrig(key)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})
	paths := safetyPaths(t)
	writePortCfg(t, paths.ConfigFile, 1964, false)
	c, err := New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.watchdogEvery = 15 * time.Millisecond
	c.watchdogFails = 2
	c.stopWatchdogLocked()
	c.healthProbe = func(context.Context, int) error { return errors.New("dead") }
	c.startWatchdogLocked()
	c.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := c.Status()
		cur, _ := mem.Snapshot(key)
		if st.Engine.State == string(dnsconfig.StateDegraded) && !dnsconfig.LooksLikeLocalhostDNS(cur) {
			if st.DNS.DnsOwnership != "NONE" && st.DNS.OwnedAdapterCount != 0 {
				t.Fatalf("ownership should be none after successful watchdog restore: %+v", st.DNS)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("watchdog restore success path timed out")
}

func TestWatchdogRestoreFailureKeepsProvenance(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{WDGFAIL-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dhcpOrig(key)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})
	mem.SilentRestoreNoop = true
	paths := safetyPaths(t)
	writePortCfg(t, paths.ConfigFile, 1965, false)
	c, err := New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.watchdogEvery = 15 * time.Millisecond
	c.watchdogFails = 2
	c.stopWatchdogLocked()
	c.healthProbe = func(context.Context, int) error { return errors.New("dead") }
	c.startWatchdogLocked()
	c.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := c.Status()
		if st.Engine.State == string(dnsconfig.StateRecoveryRequired) {
			rec, _ := c.cfgStore.Load()
			if rec == nil || len(rec.Adapters) == 0 {
				t.Fatal("provenance must be retained on restore failure")
			}
			if st.DNS.LastRestoreError == "" && st.Engine.LastError == "" {
				t.Fatal("lastRestoreError/lastError should be populated")
			}
			cur, _ := mem.Snapshot(key)
			if !dnsconfig.LooksLikeLocalhostDNS(cur) {
				t.Fatal("silent noop should leave localhost")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("watchdog restore failure path timed out")
}

func TestReevaluateDeniedDuringStoppingBarrier(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{STOPBAR-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dhcpOrig(key)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{ethAdapter(key)}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})
	paths := safetyPaths(t)
	writePortCfg(t, paths.ConfigFile, 1966, false)
	c, err := New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Enable(ctx); err != nil {
		t.Fatal(err)
	}

	restoreStarted := make(chan struct{})
	restoreContinue := make(chan struct{})
	blocking := &blockingRestore{MemoryConfigurator: mem, started: restoreStarted, cont: restoreContinue}
	c.mu.Lock()
	c.dns = blocking
	c.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = c.Disable(ctx)
	}()
	<-restoreStarted

	key2 := dnsconfig.AdapterKey{GUID: "{STOPBAR-BBBB-CCCC-DDDD-FFFFFFFFFFFF}"}
	orig2 := dnsconfig.AdapterDNSSnapshot{Key: key2, IPv4Servers: dnsconfig.DNSServerList{"9.9.9.9"}}
	dnsconfig.FillChecksum(&orig2)
	mem.AddAdapter(dnsconfig.NetworkAdapter{
		Key: key2, FriendlyName: "Wi-Fi", Description: "Intel Wi-Fi",
		IfType: dnsconfig.IfTypeIEEE80211, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"192.168.1.20"},
	}, orig2)
	before := mem.ApplyCalls
	// Concurrent reevaluate while Disable holds STOPPING + restore in progress.
	done := make(chan struct{})
	go func() {
		_ = c.ReevaluateAdapters(ctx)
		close(done)
	}()
	close(restoreContinue)
	<-done
	wg.Wait()
	if mem.ApplyCalls != before {
		t.Fatalf("ReevaluateAdapters applied DNS during Disable/STOPPING")
	}
}

type blockingRestore struct {
	*dnsconfig.MemoryConfigurator
	started chan struct{}
	cont    chan struct{}
	once    sync.Once
}

func (b *blockingRestore) Restore(key dnsconfig.AdapterKey, original dnsconfig.AdapterDNSSnapshot) error {
	b.once.Do(func() { close(b.started) })
	<-b.cont
	return b.MemoryConfigurator.Restore(key, original)
}

func TestBeginOwnershipRejectsLocalhostOriginal(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{BADORIG-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	bad := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	_, err := dnsconfig.BeginOwnership(key, bad, "s", 1)
	if err == nil {
		t.Fatal("expected reject")
	}
}
