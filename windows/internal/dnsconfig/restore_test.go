package dnsconfig_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

func TestIsEligibleConservative(t *testing.T) {
	eth := dnsconfig.NetworkAdapter{
		Key: dnsconfig.AdapterKey{GUID: "{AAAA}"}, FriendlyName: "Ethernet",
		Description: "Intel Ethernet", IfType: dnsconfig.IfTypeEthernetCSMACD,
		OperStatus: dnsconfig.OperStatusUp, IPv4Addrs: []string{"192.168.1.10"},
	}
	if !dnsconfig.IsEligible(eth) {
		t.Fatal("ethernet should be eligible")
	}
	vpn := eth
	vpn.FriendlyName = "WireGuard Tunnel"
	vpn.Description = "WireGuard Tunnel"
	vpn.IfType = dnsconfig.IfTypeTunnel
	if dnsconfig.IsEligible(vpn) {
		t.Fatal("tunnel must not be eligible")
	}
	wsl := eth
	wsl.FriendlyName = "vEthernet (WSL)"
	wsl.Description = "Hyper-V Virtual Ethernet Adapter"
	if dnsconfig.IsEligible(wsl) {
		t.Fatal("WSL must not be eligible")
	}
	down := eth
	down.OperStatus = dnsconfig.OperStatusDown
	if dnsconfig.IsEligible(down) {
		t.Fatal("down adapter not eligible")
	}
}

func TestCompareAndRestoreOwnership(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{ETH}"}
	orig := dnsconfig.AdapterDNSSnapshot{
		Key: key, IPv4Servers: dnsconfig.DNSServerList{"1.1.1.1"}, IPv6Servers: nil,
	}
	dnsconfig.FillChecksum(&orig)
	adapters := []dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Eth", IfType: dnsconfig.IfTypeEthernetCSMACD,
		OperStatus: dnsconfig.OperStatusUp, IPv4Addrs: []string{"10.0.0.2"},
	}}
	mem := dnsconfig.NewMemoryConfigurator(adapters, map[string]dnsconfig.AdapterDNSSnapshot{
		key.GUID: orig,
	})
	if err := mem.ApplyLocalhost(key); err != nil {
		t.Fatal(err)
	}
	own := dnsconfig.AdapterOwnership{
		Key: key, Original: orig, Applied: dnsconfig.LocalhostApplied, UpdatedAt: time.Now(),
	}

	// Owned → restore
	cur, _ := mem.Snapshot(key)
	r := dnsconfig.CompareAndRestore(mem, own, cur, true)
	if r.Decision != dnsconfig.RestoreApplied {
		t.Fatalf("want restored, got %s (%s)", r.Decision, r.Detail)
	}
	cur, _ = mem.Snapshot(key)
	if !dnsconfig.EqualServers(cur.IPv4Servers, orig.IPv4Servers) {
		t.Fatalf("dns not restored: %v", cur.IPv4Servers)
	}

	// Re-apply then external change → skip
	_ = mem.ApplyLocalhost(key)
	mem.SetCurrent(key, dnsconfig.AdapterDNSSnapshot{
		Key: key, IPv4Servers: dnsconfig.DNSServerList{"8.8.8.8"},
	})
	cur, _ = mem.Snapshot(key)
	r = dnsconfig.CompareAndRestore(mem, own, cur, true)
	if r.Decision != dnsconfig.RestoreSkipped {
		t.Fatalf("want skipped, got %s", r.Decision)
	}
	cur, _ = mem.Snapshot(key)
	if cur.IPv4Servers[0] != "8.8.8.8" {
		t.Fatal("must not overwrite external DNS")
	}

	// Missing adapter
	r = dnsconfig.CompareAndRestore(mem, own, dnsconfig.AdapterDNSSnapshot{}, false)
	if r.Decision != dnsconfig.RestoreMissing {
		t.Fatalf("want missing, got %s", r.Decision)
	}
}

func TestReconcileRecoveryCrash(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{ETH}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"9.9.9.9"}}
	dnsconfig.FillChecksum(&orig)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Eth", IfType: dnsconfig.IfTypeEthernetCSMACD,
		OperStatus: dnsconfig.OperStatusUp, IPv4Addrs: []string{"10.0.0.2"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: {
		Key: key, IPv4Servers: dnsconfig.LocalhostApplied.IPv4Servers,
		IPv6Servers: dnsconfig.LocalhostApplied.IPv6Servers,
	}})
	st := &dnsconfig.RecoveryState{
		Version: dnsconfig.RecoveryStateVersion, SessionID: "sess-1",
		Controller: dnsconfig.StateRecoveryRequired, Dirty: true,
		Adapters: []dnsconfig.AdapterOwnership{{
			Key: key, Original: orig, Applied: dnsconfig.LocalhostApplied,
		}},
	}
	results, err := dnsconfig.ReconcileRecovery(mem, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Decision != dnsconfig.RestoreApplied {
		t.Fatalf("unexpected results: %+v", results)
	}
	if st.Dirty || st.Controller != dnsconfig.StateDisabled {
		t.Fatalf("state not cleaned: dirty=%v controller=%s", st.Dirty, st.Controller)
	}
	cur, _ := mem.Snapshot(key)
	if cur.IPv4Servers[0] != "9.9.9.9" {
		t.Fatal("crash recovery did not restore")
	}
}

func TestStateStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := dnsconfig.NewStateStore(filepath.Join(dir, "recovery.json"))
	st := &dnsconfig.RecoveryState{
		Version: 1, SessionID: "abc", Controller: dnsconfig.StateActive, Dirty: true,
	}
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded == nil || loaded.SessionID != "abc" {
		t.Fatalf("load failed: %v %+v", err, loaded)
	}
}
