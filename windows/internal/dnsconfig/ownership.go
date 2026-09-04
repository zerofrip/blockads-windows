package dnsconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ClassifyDNS classifies current adapter DNS at ownership snapshot boundaries.
func ClassifyDNS(s AdapterDNSSnapshot) DNSClass {
	if LooksLikeBlockAdsLocalhost(s) || LooksLikeBlockAdsLocalhostIPv6Only(s) {
		return DNSClassSuspectLocalhostOrphan
	}
	if EqualServers(s.IPv4Servers, LocalhostApplied.IPv4Servers) {
		return DNSClassBlockAdsApplied
	}
	if s.IPv4DHCP || len(s.IPv4Servers) == 0 {
		return DNSClassDHCPAutomatic
	}
	return DNSClassUpstreamStatic
}

// LooksLikeBlockAdsLocalhostIPv6Only is true when IPv4 is empty/DHCP but IPv6 is ::1 only.
func LooksLikeBlockAdsLocalhostIPv6Only(s AdapterDNSSnapshot) bool {
	if len(s.IPv4Servers) > 0 {
		return false
	}
	return EqualServers(s.IPv6Servers, LocalhostApplied.IPv6Servers)
}

// LooksLikeLocalhostDNS is true for 127.0.0.1 and/or ::1 BlockAds-shaped localhost.
func LooksLikeLocalhostDNS(s AdapterDNSSnapshot) bool {
	return LooksLikeBlockAdsLocalhost(s) || LooksLikeBlockAdsLocalhostIPv6Only(s)
}

// CanBecomeOriginal reports whether a snapshot may be recorded as OriginalDns.
// BlockAds-applied / suspect localhost MUST NOT become a restorable Original.
func CanBecomeOriginal(s AdapterDNSSnapshot) bool {
	switch ClassifyDNS(s) {
	case DNSClassSuspectLocalhostOrphan, DNSClassBlockAdsApplied:
		return false
	default:
		return true
	}
}

// BeginOwnership creates an APPLYING record. Original must pass CanBecomeOriginal.
func BeginOwnership(key AdapterKey, original AdapterDNSSnapshot, sessionID string, generation int64) (AdapterOwnership, error) {
	if !CanBecomeOriginal(original) {
		return AdapterOwnership{}, fmt.Errorf("DNS-2: refusing OriginalDns class=%s (SUSPECT_LOCALHOST_ORPHAN / BlockAds-applied)", ClassifyDNS(original))
	}
	now := time.Now().UTC()
	own := AdapterOwnership{
		Key:             key,
		Original:        original,
		Applied:         LocalhostApplied,
		Phase:           OwnershipApplying,
		Provenance:      ProvenanceOwned,
		SessionID:       sessionID,
		Generation:      generation,
		OwnedAt:         now,
		LastConfirmedAt: now,
		StateVersion:    OwnershipStateVersion,
		UpdatedAt:       now,
	}
	own.RecordChecksum = OwnershipChecksum(own)
	return own, nil
}

// MarkOwned transitions APPLYING → OWNED after successful guarded Apply + readback.
func (o *AdapterOwnership) MarkOwned() error {
	if o.Phase != OwnershipApplying && o.Phase != OwnershipOwned {
		return fmt.Errorf("invalid ownership transition %s → OWNED", o.Phase)
	}
	o.Phase = OwnershipOwned
	o.Provenance = ProvenanceOwned
	o.LastConfirmedAt = time.Now().UTC()
	o.UpdatedAt = o.LastConfirmedAt
	o.RecordChecksum = OwnershipChecksum(*o)
	return nil
}

// MarkRestoring transitions OWNED → RESTORING.
func (o *AdapterOwnership) MarkRestoring() error {
	if o.Phase == "" {
		o.Phase = OwnershipOwned // legacy journals
	}
	if o.Phase != OwnershipOwned && o.Phase != OwnershipRecoveryRequired {
		return fmt.Errorf("invalid ownership transition %s → RESTORING", o.Phase)
	}
	o.Phase = OwnershipRestoring
	o.UpdatedAt = time.Now().UTC()
	o.RecordChecksum = OwnershipChecksum(*o)
	return nil
}

// MarkRecoveryRequired transitions to RECOVERY_REQUIRED (provenance retained).
func (o *AdapterOwnership) MarkRecoveryRequired() {
	o.Phase = OwnershipRecoveryRequired
	o.UpdatedAt = time.Now().UTC()
	o.RecordChecksum = OwnershipChecksum(*o)
}

// FindOwnership returns the ownership record for a stable GUID, if any.
func FindOwnership(st *RecoveryState, key AdapterKey) *AdapterOwnership {
	if st == nil {
		return nil
	}
	for i := range st.Adapters {
		if st.Adapters[i].Key.SameIdentity(key) {
			return &st.Adapters[i]
		}
	}
	return nil
}

// ClassifyProvenance decides OWNED / PROBABLE_OWNED / UNPROVEN_LOCALHOST for a localhost adapter.
func ClassifyProvenance(st *RecoveryState, key AdapterKey, current AdapterDNSSnapshot) ProvenanceLevel {
	if !LooksLikeLocalhostDNS(current) {
		return ""
	}
	if own := FindOwnership(st, key); own != nil {
		switch own.Phase {
		case OwnershipOwned, OwnershipApplying, OwnershipRestoring, OwnershipRecoveryRequired:
			if own.Provenance == ProvenanceOwned || own.Provenance == ProvenanceProbableOwned {
				return own.Provenance
			}
			return ProvenanceOwned
		}
		if own.RecordChecksum != "" && own.SessionID != "" {
			return ProvenanceProbableOwned
		}
	}
	return ProvenanceUnprovenLocalhost
}

// OwnershipChecksum fingerprints durable ownership provenance (excludes friendly display).
func OwnershipChecksum(o AdapterOwnership) string {
	h := sha256.New()
	fmt.Fprintf(h, "guid=%s;luid=%d;sess=%s;gen=%d;phase=%s;prov=%s;ov4=%s;od4=%v;av4=%s;sv=%d",
		o.Key.GUID, o.Key.LUID, o.SessionID, o.Generation, o.Phase, o.Provenance,
		strings.Join(o.Original.IPv4Servers, ","), o.Original.IPv4DHCP,
		strings.Join(o.Applied.IPv4Servers, ","), o.StateVersion)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// AllowedOwnershipTransition documents valid phase edges.
func AllowedOwnershipTransition(from, to OwnershipPhase) bool {
	switch from {
	case OwnershipUnowned:
		return to == OwnershipApplying
	case OwnershipApplying:
		return to == OwnershipOwned || to == OwnershipUnowned || to == OwnershipRecoveryRequired
	case OwnershipOwned:
		return to == OwnershipRestoring || to == OwnershipRecoveryRequired
	case OwnershipRestoring:
		return to == OwnershipUnowned || to == OwnershipRecoveryRequired
	case OwnershipRecoveryRequired:
		return to == OwnershipRestoring || to == OwnershipUnowned // unowned only via verified restore/emergency
	default:
		return false
	}
}
