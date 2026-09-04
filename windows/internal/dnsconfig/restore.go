package dnsconfig

import "fmt"

// CompareAndRestore restores original DNS only when current still equals Applied.
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
		res.Detail = fmt.Sprintf("current DNS differs from BlockAds-applied (ownership lost); leaving in place")
		return res
	}
	if err := cfg.Restore(own.Key, own.Original); err != nil {
		res.Decision = RestoreFailed
		res.Detail = err.Error()
		return res
	}
	res.Decision = RestoreApplied
	res.Detail = "restored original DNS"
	return res
}

// ReconcileRecovery walks persisted ownership and restores only owned adapters.
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
	for _, own := range st.Adapters {
		a, ok := present[own.Key.GUID]
		if !ok {
			results = append(results, CompareAndRestore(cfg, own, AdapterDNSSnapshot{}, false))
			continue
		}
		snap, err := cfg.Snapshot(a.Key)
		if err != nil {
			results = append(results, RestoreResult{Key: own.Key, Decision: RestoreFailed, Detail: err.Error()})
			remaining = append(remaining, own)
			continue
		}
		r := CompareAndRestore(cfg, own, snap, true)
		results = append(results, r)
		if r.Decision == RestoreFailed || r.Decision == RestoreSkipped {
			// Keep skipped conflicts in state for visibility; drop successful/missing.
			if r.Decision == RestoreSkipped || r.Decision == RestoreFailed {
				remaining = append(remaining, own)
			}
		}
	}
	st.Adapters = remaining
	if len(remaining) == 0 {
		st.Dirty = false
		st.Controller = StateDisabled
	} else {
		st.Controller = StateDegraded
		st.Dirty = true
	}
	return results, nil
}
