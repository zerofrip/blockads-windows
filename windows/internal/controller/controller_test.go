package controller_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

func TestEnableDisableCompareAndRestore(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{
		Key: key, IPv4Servers: dnsconfig.DNSServerList{"1.1.1.1"},
	}
	dnsconfig.FillChecksum(&orig)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Ethernet", Description: "Realtek PCIe",
		IfType: dnsconfig.IfTypeEthernetCSMACD, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"192.168.1.10"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})

	dir := t.TempDir()
	c := controller.New(controller.Paths{StateFile: filepath.Join(dir, "recovery.json")}, mem)
	c.SetConfig(controller.Config{
		ListenPort:  1853, // non-privileged for tests
		DNSProtocol: "udp",
		PrimaryDNS:  "1.1.1.1",
		FallbackDNS: "1.0.0.1",
		BlockRules:  []string{"ads.test"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Enable(ctx); err != nil {
		t.Fatalf("enable: %v", err)
	}
	st := c.Status()
	if st.State != dnsconfig.StateActive {
		t.Fatalf("state=%s", st.State)
	}
	cur, _ := mem.Snapshot(key)
	if !dnsconfig.EqualServers(cur.IPv4Servers, dnsconfig.LocalhostApplied.IPv4Servers) {
		t.Fatalf("dns not applied: %v", cur.IPv4Servers)
	}

	if err := c.Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}
	cur, _ = mem.Snapshot(key)
	if !dnsconfig.EqualServers(cur.IPv4Servers, orig.IPv4Servers) {
		t.Fatalf("dns not restored: %v", cur.IPv4Servers)
	}
	if c.Status().State != dnsconfig.StateDisabled {
		t.Fatalf("want DISABLED, got %s", c.Status().State)
	}
}

func TestDisableSkipsExternallyChangedDNS(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{BBBBBBBB-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"9.9.9.9"}}
	dnsconfig.FillChecksum(&orig)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Wi-Fi", Description: "Intel Wi-Fi",
		IfType: dnsconfig.IfTypeIEEE80211, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"192.168.1.20"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})

	dir := t.TempDir()
	c := controller.New(controller.Paths{StateFile: filepath.Join(dir, "recovery.json")}, mem)
	c.SetConfig(controller.Config{ListenPort: 1854, PrimaryDNS: "1.1.1.1", DNSProtocol: "udp"})
	ctx := context.Background()
	if err := c.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	mem.SetCurrent(key, dnsconfig.AdapterDNSSnapshot{
		Key: key, IPv4Servers: dnsconfig.DNSServerList{"8.8.8.8"},
	})
	if err := c.Disable(); err != nil {
		t.Fatal(err)
	}
	cur, _ := mem.Snapshot(key)
	if cur.IPv4Servers[0] != "8.8.8.8" {
		t.Fatalf("external DNS overwritten: %v", cur.IPv4Servers)
	}
}

func TestCrashRecoveryOwnedOnly(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{CCCCCCCC-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"4.4.4.4"}}
	dnsconfig.FillChecksum(&orig)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Ethernet 2", Description: "Broadcom",
		IfType: dnsconfig.IfTypeEthernetCSMACD, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"10.0.0.5"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: {
		Key: key, IPv4Servers: dnsconfig.LocalhostApplied.IPv4Servers,
		IPv6Servers: dnsconfig.LocalhostApplied.IPv6Servers,
	}})

	dir := t.TempDir()
	store := dnsconfig.NewStateStore(filepath.Join(dir, "recovery.json"))
	_ = store.Save(&dnsconfig.RecoveryState{
		Version: 1, SessionID: "dead", Dirty: true,
		Controller: dnsconfig.StateRecoveryRequired,
		Adapters: []dnsconfig.AdapterOwnership{{
			Key: key, Original: orig, Applied: dnsconfig.LocalhostApplied,
		}},
	})

	c := controller.New(controller.Paths{StateFile: store.Path()}, mem)
	results, err := c.RecoverIfNeeded()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Decision != dnsconfig.RestoreApplied {
		t.Fatalf("results=%+v", results)
	}
	cur, _ := mem.Snapshot(key)
	if cur.IPv4Servers[0] != "4.4.4.4" {
		t.Fatal("recovery failed")
	}
}
