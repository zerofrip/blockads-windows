package dnsconfig

import "strings"

// IfType constants (subset of IpIfOperStatus / IANA ifType).
const (
	IfTypeOther              = 1
	IfTypeEthernetCSMACD     = 6
	IfTypeSoftwareLoopback   = 24
	IfTypePPP                = 23
	IfTypeIEEE80211          = 71
	IfTypeTunnel             = 131
	IfTypePropVirtual        = 53
	IfTypeWWANPP             = 243
	IfTypeWWANPP2            = 244
)

const (
	OperStatusUp             = 1
	OperStatusDown           = 2
	OperStatusTesting        = 3
	OperStatusUnknown        = 4
	OperStatusDormant        = 5
	OperStatusNotPresent     = 6
	OperStatusLowerLayerDown = 7
)

// denylistKeywords — case-insensitive match against description/friendly name.
// Note: "Microsoft Hyper-V Network Adapter" is the synthetic NIC *inside* a
// Hyper-V guest and MUST remain eligible (see isHyperVGuestNic). Host-side
// vEthernet / "Hyper-V Virtual Ethernet Adapter" stay denylisted.
var denylistKeywords = []string{
	"wintun", "wireguard", "vethernet", "wsl", "docker",
	"virtualbox", "vmware", "tap-windows", "tap-win", "openvpn", "nordlynx",
	"zerotier", "tailscale", "cloudflare warp", "warp", "blockads",
	"virtual ethernet", "default switch",
	"vpn",
}

// isHyperVGuestNic reports the in-guest Hyper-V synthetic Ethernet adapter
// (netvsc), which is the real primary NIC for disposable Hyper-V VMs.
func isHyperVGuestNic(blob string) bool {
	return strings.Contains(blob, "microsoft hyper-v network adapter") &&
		!strings.Contains(blob, "virtual ethernet")
}

// IsEligible reports whether an adapter may receive BlockAds DNS (conservative).
// Policy version: AdapterPolicyVersion.
func IsEligible(a NetworkAdapter) bool {
	if a.OperStatus != OperStatusUp {
		return false
	}
	if a.IfType == IfTypeSoftwareLoopback {
		return false
	}
	if a.IfType == IfTypeTunnel || a.IfType == IfTypePropVirtual || a.IfType == IfTypePPP {
		return false
	}
	// Phase 2: Ethernet + Wi-Fi only
	if a.IfType != IfTypeEthernetCSMACD && a.IfType != IfTypeIEEE80211 {
		return false
	}
	blob := strings.ToLower(a.FriendlyName + " " + a.Description)
	if !isHyperVGuestNic(blob) {
		for _, kw := range denylistKeywords {
			if strings.Contains(blob, kw) {
				return false
			}
		}
		// Host Hyper-V / legacy keyword coverage without excluding guest NIC.
		if strings.Contains(blob, "hyper-v") {
			return false
		}
		if strings.Contains(blob, "virtual") {
			return false
		}
	}
	if len(a.IPv4Addrs) == 0 && len(a.IPv6Addrs) == 0 {
		return false
	}
	if a.Key.GUID == "" {
		return false
	}
	return true
}

// FilterEligible returns adapters that pass IsEligible.
func FilterEligible(adapters []NetworkAdapter) []NetworkAdapter {
	out := make([]NetworkAdapter, 0, len(adapters))
	for _, a := range adapters {
		if IsEligible(a) {
			out = append(out, a)
		}
	}
	return out
}

