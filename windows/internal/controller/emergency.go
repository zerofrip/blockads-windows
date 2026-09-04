package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

// EmergencyOptions controls emergency-restore scope.
type EmergencyOptions struct {
	// ForceUnprovenLocalhost resets UNPROVEN_LOCALHOST adapters to DHCP.
	// Default false — never the default path.
	ForceUnprovenLocalhost bool
}

// EmergencyAdapterPlan describes intended action before mutation.
type EmergencyAdapterPlan struct {
	GUID        string                   `json:"guid"`
	DisplayName string                   `json:"displayName,omitempty"`
	Provenance  dnsconfig.ProvenanceLevel `json:"provenance"`
	Class       dnsconfig.DNSClass       `json:"dnsClass"`
	Action      string                   `json:"action"` // restore_proven|skip_unproven|force_unproven|skip_other
	Detail      string                   `json:"detail,omitempty"`
}

// EmergencyRestoreResult is the report from emergency-restore.
type EmergencyRestoreResult struct {
	Plan    []EmergencyAdapterPlan     `json:"plan"`
	Actions []dnsconfig.RestoreResult  `json:"actions"`
	Note    string                     `json:"note"`
}

// EmergencyRestore compare-and-restores proven BlockAds-owned DNS only by default.
// Does not start filtering, bind port 53, download filters, or require DoH.
func EmergencyRestore(paths Paths, dnsCfg dnsconfig.DnsConfigurator, opts EmergencyOptions) (EmergencyRestoreResult, error) {
	store := dnsconfig.NewStateStore(paths.StateFile)
	res := EmergencyRestoreResult{
		Note: "emergency-restore: proven ownership only by default; no filter/DoH/listener",
	}
	if opts.ForceUnprovenLocalhost {
		res.Note += "; --force-unproven-localhost enabled"
	}

	st, err := store.Load()
	if err != nil {
		return res, err
	}

	adapters, err := dnsCfg.ListAdapters()
	if err != nil {
		return res, err
	}
	names := map[string]string{}
	for _, a := range adapters {
		names[a.Key.GUID] = a.FriendlyName
	}

	// Plan first (what we intend).
	for _, a := range adapters {
		snap, err := dnsCfg.Snapshot(a.Key)
		if err != nil {
			continue
		}
		class := dnsconfig.ClassifyDNS(snap)
		prov := dnsconfig.ClassifyProvenance(st, a.Key, snap)
		plan := EmergencyAdapterPlan{
			GUID: a.Key.GUID, DisplayName: names[a.Key.GUID],
			Provenance: prov, Class: class,
		}
		switch {
		case prov == dnsconfig.ProvenanceOwned || prov == dnsconfig.ProvenanceProbableOwned:
			plan.Action = "restore_proven"
			plan.Detail = "BlockAds provenance present; compare-and-restore"
		case prov == dnsconfig.ProvenanceUnprovenLocalhost:
			if opts.ForceUnprovenLocalhost {
				plan.Action = "force_unproven"
				plan.Detail = "explicit --force-unproven-localhost"
			} else {
				plan.Action = "skip_unproven"
				plan.Detail = "localhost without BlockAds provenance; not mutated"
			}
		default:
			plan.Action = "skip_other"
			plan.Detail = "not BlockAds localhost"
		}
		res.Plan = append(res.Plan, plan)
	}

	// Execute proven ownership restore via journal.
	if st != nil && len(st.Adapters) > 0 {
		results, err := dnsconfig.ReconcileRecovery(dnsCfg, st)
		if err != nil {
			return res, err
		}
		res.Actions = append(res.Actions, results...)
		if len(st.Adapters) == 0 {
			_ = store.Clear()
		} else {
			st.SavedAt = time.Now().UTC()
			_ = store.Save(st)
		}
	}

	if !opts.ForceUnprovenLocalhost {
		return res, nil
	}

	// Forced path only: reset remaining unproven localhost to DHCP.
	st, _ = store.Load()
	for _, a := range adapters {
		snap, err := dnsCfg.Snapshot(a.Key)
		if err != nil || !dnsconfig.LooksLikeLocalhostDNS(snap) {
			continue
		}
		prov := dnsconfig.ClassifyProvenance(st, a.Key, snap)
		if prov != dnsconfig.ProvenanceUnprovenLocalhost {
			continue
		}
		dhcp := dnsconfig.AdapterDNSSnapshot{Key: a.Key, IPv4DHCP: true, IPv6DHCP: true}
		if err := dnsCfg.Restore(a.Key, dhcp); err != nil {
			res.Actions = append(res.Actions, dnsconfig.RestoreResult{
				Key: a.Key, Decision: dnsconfig.RestoreFailed, Detail: err.Error(),
			})
			continue
		}
		after, err := dnsCfg.Snapshot(a.Key)
		if err != nil || dnsconfig.LooksLikeLocalhostDNS(after) {
			detail := "force-unproven clear failed verification"
			if err != nil {
				detail = err.Error()
			}
			res.Actions = append(res.Actions, dnsconfig.RestoreResult{
				Key: a.Key, Decision: dnsconfig.RestoreFailed, Detail: detail,
			})
			continue
		}
		res.Actions = append(res.Actions, dnsconfig.RestoreResult{
			Key: a.Key, Decision: dnsconfig.RestoreApplied,
			Detail: "forced clear of UNPROVEN_LOCALHOST to DHCP",
		})
	}
	return res, nil
}

// PrintEmergencyRestore writes a JSON report to stdout.
func PrintEmergencyRestore(paths Paths, dnsCfg dnsconfig.DnsConfigurator, opts EmergencyOptions) error {
	res, err := EmergencyRestore(paths, dnsCfg, opts)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	if err != nil {
		return err
	}
	for _, a := range res.Actions {
		if a.Decision == dnsconfig.RestoreFailed {
			return fmt.Errorf("emergency-restore had failures")
		}
	}
	return nil
}
