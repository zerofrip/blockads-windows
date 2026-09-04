package dnsconfig

import "time"

// EngineState is the controller lifecycle state.
type EngineState string

const (
	StateDisabled         EngineState = "DISABLED"
	StateStarting         EngineState = "STARTING"
	StateActive           EngineState = "ACTIVE"
	StateStopping         EngineState = "STOPPING"
	StateRecoveryRequired EngineState = "RECOVERY_REQUIRED"
	StateDegraded         EngineState = "DEGRADED"
)

// AdapterKey is a stable interface identity (never display name).
type AdapterKey struct {
	GUID string `json:"guid"`
	LUID uint64 `json:"luid,omitempty"`
}

// DNSServerList is an ordered list of DNS server addresses (IPv4/IPv6 text).
type DNSServerList []string

// AdapterDNSSnapshot is one adapter's DNS configuration at a point in time.
type AdapterDNSSnapshot struct {
	Key            AdapterKey    `json:"key"`
	FriendlyName   string        `json:"friendlyName,omitempty"` // display only
	IPv4Servers    DNSServerList `json:"ipv4Servers"`
	IPv6Servers    DNSServerList `json:"ipv6Servers"`
	IPv4DHCP       bool          `json:"ipv4Dhcp"`
	IPv6DHCP       bool          `json:"ipv6Dhcp"`
	Checksum       string        `json:"checksum"`
}

// AppliedDNS is what BlockAds wrote to an adapter.
type AppliedDNS struct {
	IPv4Servers DNSServerList `json:"ipv4Servers"`
	IPv6Servers DNSServerList `json:"ipv6Servers"`
}

// AdapterOwnership records original vs BlockAds-applied DNS for compare-and-restore.
type AdapterOwnership struct {
	Key       AdapterKey         `json:"key"`
	Original  AdapterDNSSnapshot `json:"original"`
	Applied   AppliedDNS         `json:"applied"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

// RecoveryState is persisted across crashes. Restoring is ownership-verified, never blind.
type RecoveryState struct {
	Version       int                `json:"version"`
	PolicyVersion int                `json:"policyVersion"`
	SessionID     string             `json:"sessionId"`
	Controller    EngineState        `json:"controller"`
	ListenAddr    string             `json:"listenAddr"`
	ListenPort    int                `json:"listenPort"`
	Adapters      []AdapterOwnership `json:"adapters"`
	SavedAt       time.Time          `json:"savedAt"`
	Dirty         bool               `json:"dirty"`
}

const RecoveryStateVersion = 1
const AdapterPolicyVersion = 1

// LocalhostApplied is the DNS BlockAds sets on eligible adapters.
var LocalhostApplied = AppliedDNS{
	IPv4Servers: DNSServerList{"127.0.0.1"},
	IPv6Servers: DNSServerList{"::1"},
}

// EqualServers compares ordered server lists.
func EqualServers(a, b DNSServerList) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// MatchesApplied reports whether current snapshot equals what this session applied.
func (o AdapterOwnership) MatchesApplied(current AdapterDNSSnapshot) bool {
	return EqualServers(current.IPv4Servers, o.Applied.IPv4Servers) &&
		EqualServers(current.IPv6Servers, o.Applied.IPv6Servers)
}
