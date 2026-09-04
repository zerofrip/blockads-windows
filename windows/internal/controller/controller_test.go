package controller_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

func testPaths(t *testing.T) controller.Paths {
	t.Helper()
	dir := t.TempDir()
	return controller.Paths{
		DataDir:    dir,
		StateFile:  filepath.Join(dir, "recovery.json"),
		ConfigFile: filepath.Join(dir, "config.json"),
		FilterDir:  filepath.Join(dir, "filters"),
	}
}

func writeHighPortConfig(t *testing.T, path string, port int) {
	t.Helper()
	content := fmt.Sprintf(`{
  "version": 1,
  "enabled": false,
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
}`, port)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnableDisableWithHighPort(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"1.1.1.1"}}
	dnsconfig.FillChecksum(&orig)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Ethernet", Description: "Realtek PCIe",
		IfType: dnsconfig.IfTypeEthernetCSMACD, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"192.168.1.10"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})

	paths := testPaths(t)
	writeHighPortConfig(t, paths.ConfigFile, 1853)

	c, err := controller.New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.Enable(ctx); err != nil {
		t.Fatalf("enable: %v", err)
	}
	st := c.Status()
	if !st.Engine.FilteringEnabled {
		t.Fatalf("expected filtering enabled: %+v", st)
	}
	cur, _ := mem.Snapshot(key)
	if !dnsconfig.EqualServers(cur.IPv4Servers, dnsconfig.LocalhostApplied.IPv4Servers) {
		t.Fatalf("dns not applied: %v", cur.IPv4Servers)
	}
	if err := c.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	cur, _ = mem.Snapshot(key)
	if !dnsconfig.EqualServers(cur.IPv4Servers, orig.IPv4Servers) {
		t.Fatalf("dns not restored: %v", cur.IPv4Servers)
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

	paths := testPaths(t)
	writeHighPortConfig(t, paths.ConfigFile, 1854)
	c, err := controller.New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	mem.SetCurrent(key, dnsconfig.AdapterDNSSnapshot{
		Key: key, IPv4Servers: dnsconfig.DNSServerList{"8.8.8.8"},
	})
	if err := c.Disable(ctx); err != nil {
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

	paths := testPaths(t)
	store := dnsconfig.NewStateStore(paths.StateFile)
	_ = store.Save(&dnsconfig.RecoveryState{
		Version: 1, SessionID: "dead", Dirty: true,
		Controller: dnsconfig.StateRecoveryRequired,
		Adapters: []dnsconfig.AdapterOwnership{{
			Key: key, Original: orig, Applied: dnsconfig.LocalhostApplied,
		}},
	})

	c, err := controller.New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	results, err := c.Recover(context.Background())
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
