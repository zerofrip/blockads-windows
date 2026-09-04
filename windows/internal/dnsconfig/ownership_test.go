package dnsconfig_test

import (
	"testing"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

func TestClassifyDNSAndCanBecomeOriginal(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{CLASS---BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	local := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	if dnsconfig.ClassifyDNS(local) != dnsconfig.DNSClassSuspectLocalhostOrphan {
		t.Fatal(dnsconfig.ClassifyDNS(local))
	}
	if dnsconfig.CanBecomeOriginal(local) {
		t.Fatal("localhost must not become Original")
	}
	dhcp := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4DHCP: true}
	if dnsconfig.ClassifyDNS(dhcp) != dnsconfig.DNSClassDHCPAutomatic {
		t.Fatal(dnsconfig.ClassifyDNS(dhcp))
	}
	if !dnsconfig.CanBecomeOriginal(dhcp) {
		t.Fatal("DHCP should be valid Original")
	}
}

func TestOwnershipStateMachineForbiddenEdges(t *testing.T) {
	if dnsconfig.AllowedOwnershipTransition(dnsconfig.OwnershipUnowned, dnsconfig.OwnershipOwned) {
		t.Fatal("UNOWNED→OWNED forbidden without APPLYING")
	}
	if dnsconfig.AllowedOwnershipTransition(dnsconfig.OwnershipRecoveryRequired, dnsconfig.OwnershipOwned) {
		t.Fatal("RECOVERY_REQUIRED→OWNED forbidden")
	}
	if !dnsconfig.AllowedOwnershipTransition(dnsconfig.OwnershipUnowned, dnsconfig.OwnershipApplying) {
		t.Fatal("UNOWNED→APPLYING required")
	}
}

func TestVerifiedRestoreSilentNoopFailsAndRetains(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{VERIFY--BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4DHCP: true}
	dnsconfig.FillChecksum(&orig)
	local := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	dnsconfig.FillChecksum(&local)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Ethernet", Description: "Realtek",
		IfType: dnsconfig.IfTypeEthernetCSMACD, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"192.168.1.10"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: local})
	mem.SilentRestoreNoop = true
	own, err := dnsconfig.BeginOwnership(key, orig, "s", 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = own.MarkOwned()
	r := dnsconfig.CompareAndRestore(mem, own, local, true)
	if r.Decision != dnsconfig.RestoreFailed {
		t.Fatalf("got %s %s", r.Decision, r.Detail)
	}
	st := &dnsconfig.RecoveryState{Adapters: []dnsconfig.AdapterOwnership{own}, Dirty: true}
	_, _ = dnsconfig.ReconcileRecovery(mem, st)
	if len(st.Adapters) != 1 {
		t.Fatal("provenance must remain after failed verified restore")
	}
	if st.Controller != dnsconfig.StateRecoveryRequired {
		t.Fatalf("controller=%s", st.Controller)
	}
}

func TestVerifiedRestoreAPIFailure(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{APIFAIL-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"1.1.1.1"}}
	dnsconfig.FillChecksum(&orig)
	local := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	dnsconfig.FillChecksum(&local)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Ethernet", Description: "Realtek",
		IfType: dnsconfig.IfTypeEthernetCSMACD, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"192.168.1.10"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: local})
	mem.Fails[key.GUID] = errInjected
	own := dnsconfig.AdapterOwnership{
		Key: key, Original: orig, Applied: dnsconfig.LocalhostApplied,
		Phase: dnsconfig.OwnershipOwned, Provenance: dnsconfig.ProvenanceOwned, UpdatedAt: time.Now().UTC(),
	}
	r := dnsconfig.CompareAndRestore(mem, own, local, true)
	if r.Decision != dnsconfig.RestoreFailed {
		t.Fatalf("got %s", r.Decision)
	}
}

var errInjected = errString("injected")

type errString string

func (e errString) Error() string { return string(e) }

func TestMatchesRestoredSemanticDHCP(t *testing.T) {
	orig := dnsconfig.AdapterDNSSnapshot{IPv4DHCP: true}
	afterEmpty := dnsconfig.AdapterDNSSnapshot{IPv4DHCP: true}
	if !dnsconfig.MatchesRestoredSemantic(orig, afterEmpty) {
		t.Fatal("DHCP empty should match")
	}
	afterLocal := dnsconfig.AdapterDNSSnapshot{IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}}
	if dnsconfig.MatchesRestoredSemantic(orig, afterLocal) {
		t.Fatal("localhost must not match restored DHCP")
	}
}

func TestReconcileRetainsOwnershipWhenAdapterMissing(t *testing.T) {
	key := dnsconfig.AdapterKey{GUID: "{MISSING-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"1.1.1.1", "8.8.8.8"}}
	dnsconfig.FillChecksum(&orig)
	own, err := dnsconfig.BeginOwnership(key, orig, "s", 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = own.MarkOwned()
	// Empty adapter list simulates reboot with NIC admin-down.
	mem := dnsconfig.NewMemoryConfigurator(nil, map[string]dnsconfig.AdapterDNSSnapshot{
		key.GUID: {Key: key, IPv4Servers: dnsconfig.DNSServerList{"127.0.0.1"}},
	})
	st := &dnsconfig.RecoveryState{Adapters: []dnsconfig.AdapterOwnership{own}, Dirty: true}
	results, err := dnsconfig.ReconcileRecovery(mem, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Decision != dnsconfig.RestoreMissing {
		t.Fatalf("results=%+v", results)
	}
	if len(st.Adapters) != 1 {
		t.Fatal("must retain ownership when adapter missing")
	}
	if !dnsconfig.EqualServers(st.Adapters[0].Original.IPv4Servers, orig.IPv4Servers) {
		t.Fatalf("Original lost: %+v", st.Adapters[0].Original)
	}
}

