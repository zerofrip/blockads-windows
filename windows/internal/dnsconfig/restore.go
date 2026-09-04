package dnsconfig

import "fmt"

// LooksLikeBlockAdsLocalhost reports whether the snapshot still points at
// BlockAds-controlled IPv4 localhost DNS (DNS-1 orphan detector).
func LooksLikeBlockAdsLocalhost(s AdapterDNSSnapshot) bool {
	return EqualServers(s.IPv4Servers, LocalhostApplied.IPv4Servers)
}

// MatchesRestoredSemantic reports whether readback matches the intended restore target.
// DHCP/automatic originals compare by automatic semantics, not only string equality,
// because Windows may represent DHCP as empty NameServer lists.
func MatchesRestoredSemantic(original, after AdapterDNSSnapshot) bool {
	if LooksLikeLocalhostDNS(after) {
		return false
	}
	if original.IPv4DHCP || len(original.IPv4Servers) == 0 {
		// Automatic/DHCP: accept DHCP flag, empty servers, or non-localhost servers
		// that are not BlockAds-applied. Prefer exact match when both are static-empty.
		if after.IPv4DHCP || len(after.IPv4Servers) == 0 {
			return true
		}
		// Some stacks refill DHCP-learned servers into the list after reset.
		return !LooksLikeLocalhostDNS(after)
	}
	return EqualServers(after.IPv4Servers, original.IPv4Servers)
}

// CompareAndRestore restores original DNS only when current still equals Applied.
// A successful SetInterfaceDnsSettings is NOT sufficient — readback must verify.
// On failure, caller MUST retain ownership provenance (never clear first).
func CompareAndRestore(
	cfg DnsConfigurator,
	own AdapterOwnership,
	current AdapterDNSSnapshot,
	adapterPresent bool,
) RestoreResult {
	res := RestoreResult{Key: own.Key}
	if !adapterPresent {
		res.Decision = RestoreMissing
		res.Detail = "adapter no longer present"
		return res
	}
	if !own.MatchesApplied(current) {
		res.Decision = RestoreSkipped
		res.Detail = "current DNS differs from BlockAds-applied (ownership lost); leaving in place"
		return res
	}
	if err := cfg.Restore(own.Key, own.Original); err != nil {
		res.Decision = RestoreFailed
		res.Detail = err.Error()
		return res
	}
	after, err := cfg.Snapshot(own.Key)
	if err != nil {
		res.Decision = RestoreFailed
		res.Detail = fmt.Sprintf("restored but re-snapshot failed: %v", err)
		return res
	}
	if !MatchesRestoredSemantic(own.Original, after) {
		res.Decision = RestoreFailed
		if LooksLikeLocalhostDNS(after) {
			res.Detail = "post-restore verification: adapter still has BlockAds localhost DNS"
		} else {
			res.Detail = fmt.Sprintf("post-restore verification: DNS mismatch want=%v dhcp=%v got=%v dhcp=%v",
				own.Original.IPv4Servers, own.Original.IPv4DHCP, after.IPv4Servers, after.IPv4DHCP)
		}
		return res
	}
	res.Decision = RestoreApplied
	res.Detail = "restored original DNS (verified readback)"
	return res
}

// ReconcileRecovery walks persisted ownership and restores only owned adapters.
// Provenance is cleared only after verified restore success.
func ReconcileRecovery(cfg DnsConfigurator, st *RecoveryState) ([]RestoreResult, error) {
	adapters, err := cfg.ListAdapters()
	if err != nil {
		return nil, err
	}
	present := make(map[string]NetworkAdapter, len(adapters))
	for _, a := range adapters {
		present[a.Key.GUID] = a
	}

	results := make([]RestoreResult, 0, len(st.Adapters))
	remaining := make([]AdapterOwnership, 0)
	for i := range st.Adapters {
		own := st.Adapters[i]
		_ = own.MarkRestoring()
		a, ok := present[own.Key.GUID]
		if !ok {
			r := CompareAndRestore(cfg, own, AdapterDNSSnapshot{}, false)
			results = append(results, r)
			// Missing adapter: drop ownership only for RestoreMissing (nothing to recover on NIC).
			continue
		}
		snap, err := cfg.Snapshot(a.Key)
		if err != nil {
			own.MarkRecoveryRequired()
			results = append(results, RestoreResult{Key: own.Key, Decision: RestoreFailed, Detail: err.Error()})
			remaining = append(remaining, own)
			continue
		}
		r := CompareAndRestore(cfg, own, snap, true)
		results = append(results, r)
		switch r.Decision {
		case RestoreApplied:
			// verified — drop provenance
		case RestoreMissing:
			// nothing
		case RestoreSkipped:
			// external change wins; release ownership without mutation
		case RestoreFailed:
			own.MarkRecoveryRequired()
			remaining = append(remaining, own)
		}
	}
	st.Adapters = remaining
	if len(remaining) == 0 {
		st.Dirty = false
		st.Controller = StateDisabled
		st.SuspectedUnprovenLocalhost = nil
	} else {
		st.Controller = StateRecoveryRequired
		st.Dirty = true
	}
	return results, nil
}
