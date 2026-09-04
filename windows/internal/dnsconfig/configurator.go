package dnsconfig

import "errors"

var (
	ErrNotSupported     = errors.New("dns configuration not supported on this platform")
	ErrPortInUse        = errors.New("DNS listen port already in use")
	ErrListenerUnhealthy = errors.New("local DNS listener failed health check")
	ErrPartialFailure   = errors.New("partial DNS configuration failure; rolled back")
)

// NetworkAdapter is an enumerated interface candidate.
type NetworkAdapter struct {
	Key          AdapterKey
	FriendlyName string
	Description  string
	IfType       uint32
	OperStatus   uint32
	IPv4Addrs    []string
	IPv6Addrs    []string
}

// DnsConfigurator snapshots and mutates per-adapter DNS using native OS APIs.
// Windows structures must not leak through this interface.
type DnsConfigurator interface {
	// ListAdapters returns adapters visible to the OS.
	ListAdapters() ([]NetworkAdapter, error)
	// Snapshot reads current DNS settings for one adapter.
	Snapshot(key AdapterKey) (AdapterDNSSnapshot, error)
	// ApplyLocalhost sets DNS to 127.0.0.1 / ::1 (as applicable) for the adapter.
	ApplyLocalhost(key AdapterKey) error
	// Restore writes a previously snapshotted configuration.
	Restore(key AdapterKey, original AdapterDNSSnapshot) error
	// Status returns a human-readable summary for debugging (no secrets).
	Status() (string, error)
}

// RestoreDecision is the result of compare-and-restore for one adapter.
type RestoreDecision string

const (
	RestoreApplied  RestoreDecision = "restored"
	RestoreSkipped  RestoreDecision = "skipped_conflict"
	RestoreMissing  RestoreDecision = "adapter_missing"
	RestoreFailed   RestoreDecision = "failed"
)

// RestoreResult is per-adapter compare-and-restore outcome.
type RestoreResult struct {
	Key      AdapterKey
	Decision RestoreDecision
	Detail   string
}
