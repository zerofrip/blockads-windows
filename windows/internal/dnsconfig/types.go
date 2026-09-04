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

// OwnershipPhase is the per-adapter DNS ownership state machine (DNS-2).
type OwnershipPhase string

const (
	OwnershipUnowned          OwnershipPhase = "UNOWNED"
	OwnershipApplying         OwnershipPhase = "APPLYING"
	OwnershipOwned            OwnershipPhase = "OWNED"
	OwnershipRestoring        OwnershipPhase = "RESTORING"
	OwnershipRecoveryRequired OwnershipPhase = "RECOVERY_REQUIRED"
)

// ProvenanceLevel classifies how strongly BlockAds can claim localhost DNS.
type ProvenanceLevel string

const (
	// ProvenanceOwned: valid journal/session proves BlockAds applied this value.
	ProvenanceOwned ProvenanceLevel = "OWNED"
	// ProvenanceProbableOwned: durable marker proves prior application but journal incomplete.
	ProvenanceProbableOwned ProvenanceLevel = "PROBABLE_OWNED"
	// ProvenanceUnprovenLocalhost: localhost present with no trustworthy BlockAds evidence.
	ProvenanceUnprovenLocalhost ProvenanceLevel = "UNPROVEN_LOCALHOST"
)

// DNSClass classifies a snapshot at ownership boundaries.
type DNSClass string

const (
	DNSClassDHCPAutomatic          DNSClass = "DHCP_AUTOMATIC"
	DNSClassUpstreamStatic         DNSClass = "UPSTREAM_STATIC"
	DNSClassBlockAdsApplied        DNSClass = "BLOCKADS_APPLIED"
	DNSClassSuspectLocalhostOrphan DNSClass = "SUSPECT_LOCALHOST_ORPHAN"
	DNSClassExternalStatic         DNSClass = "EXTERNAL_STATIC"
)

// AdapterKey is a stable interface identity (never display name alone).
type AdapterKey struct {
	GUID string `json:"guid"`
	LUID uint64 `json:"luid,omitempty"`
}

// SameIdentity reports whether two keys refer to the same stable adapter.
// GUID is authoritative; LUID is supporting when both non-zero.
func (a AdapterKey) SameIdentity(b AdapterKey) bool {
	if a.GUID == "" || b.GUID == "" {
		return false
	}
	return a.GUID == b.GUID
}

// DNSServerList is an ordered list of DNS server addresses (IPv4/IPv6 text).
type DNSServerList []string

// AdapterDNSSnapshot is one adapter's DNS configuration at a point in time.
type AdapterDNSSnapshot struct {
	Key          AdapterKey    `json:"key"`
	FriendlyName string        `json:"friendlyName,omitempty"` // display only
	IPv4Servers  DNSServerList `json:"ipv4Servers"`
	IPv6Servers  DNSServerList `json:"ipv6Servers"`
	IPv4DHCP     bool          `json:"ipv4Dhcp"`
	IPv6DHCP     bool          `json:"ipv6Dhcp"`
	Checksum     string        `json:"checksum"`
}

// AppliedDNS is what BlockAds wrote to an adapter.
type AppliedDNS struct {
	IPv4Servers DNSServerList `json:"ipv4Servers"`
	IPv6Servers DNSServerList `json:"ipv6Servers"`
}

// AdapterOwnership records original vs BlockAds-applied DNS for compare-and-restore.
//
// DNS-2: Original is immutable for the lifetime of an ownership session.
// Temporary adapter disappearance MUST NOT re-snapshot Original.
type AdapterOwnership struct {
	Key             AdapterKey         `json:"key"`
	Original        AdapterDNSSnapshot `json:"original"`
	Applied         AppliedDNS         `json:"applied"`
	Phase           OwnershipPhase     `json:"phase"`
	Provenance      ProvenanceLevel    `json:"provenance"`
	SessionID       string             `json:"sessionId,omitempty"`
	Generation      int64              `json:"generation,omitempty"`
	OwnedAt         time.Time          `json:"ownedAt,omitempty"`
	LastConfirmedAt time.Time          `json:"lastConfirmedAt,omitempty"`
	StateVersion    int                `json:"stateVersion,omitempty"`
	RecordChecksum  string             `json:"recordChecksum,omitempty"`
	UpdatedAt       time.Time          `json:"updatedAt"`
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
	// SuspectedUnprovenLocalhost lists adapter GUIDs with localhost DNS but no
	// trustworthy BlockAds provenance (diagnostic only; not auto-mutated).
	SuspectedUnprovenLocalhost []string `json:"suspectedUnprovenLocalhost,omitempty"`
}

const RecoveryStateVersion = 2
const AdapterPolicyVersion = 1
const OwnershipStateVersion = 1

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

// MatchesApplied reports whether current snapshot still equals what this session applied.
// IPv4 is authoritative. IPv6 is best-effort: Get/SetInterfaceDnsSettings often omit or
// fail IPv6 DNS on adapters without a usable IPv6 stack, so an empty current IPv6 list
// does not by itself forfeit IPv4 ownership.
func (o AdapterOwnership) MatchesApplied(current AdapterDNSSnapshot) bool {
	if !EqualServers(current.IPv4Servers, o.Applied.IPv4Servers) {
		return false
	}
	if len(o.Applied.IPv6Servers) == 0 || len(current.IPv6Servers) == 0 {
		return true
	}
	return EqualServers(current.IPv6Servers, o.Applied.IPv6Servers)
}
