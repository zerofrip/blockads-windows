package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

// SAFETY PROPERTY DNS-1:
// BlockAds MUST NOT leave a Windows adapter configured to use BlockAds-controlled
// localhost DNS unless a healthy BlockAds DNS listener is available.
// If listener health cannot be maintained, BlockAds MUST compare-and-restore DNS
// it still owns. Externally modified DNS MUST never be overwritten.
//
// SAFETY PROPERTY DNS-2:
// Once ownership is established for a stable adapter identity, OriginalDns is
// immutable for that ownership session. Temporary disappearance must not
// re-snapshot Original.

var (
	errDNSStopping   = errors.New("controller is stopping; DNS apply denied")
	errDNSNotReady   = errors.New("engine not ready for DNS apply")
	errDNSNoListener = errors.New("localhost DNS listener not running")
	errDNSUnhealthy  = errors.New("localhost DNS listener health check failed")
)

const (
	watchdogInterval  = 5 * time.Second
	watchdogFailLimit = 3
	defaultListenPort = 53
)

// HealthProbe checks that the local DNS listener answers. Tests may replace it.
type HealthProbe func(ctx context.Context, port int) error

func (c *Controller) mayApplyLocalDnsLocked(ctx context.Context) error {
	if !c.acceptMut {
		return errDNSStopping
	}
	if c.state != dnsconfig.StateActive && c.state != dnsconfig.StateStarting {
		return fmt.Errorf("%w: state=%s", errDNSNotReady, c.state)
	}
	if !c.testBypassEngine {
		if c.engine == nil || !c.engine.IsRunning() {
			return errDNSNoListener
		}
	}
	port := c.appConfig.DNS.ListenPort
	if port == 0 {
		port = defaultListenPort
	}
	probe := c.healthProbe
	if probe == nil {
		probe = healthCheckLocalDNS
	}
	if err := probe(ctx, port); err != nil {
		return fmt.Errorf("%w: %v", errDNSUnhealthy, err)
	}
	return nil
}

func (c *Controller) applyLocalhostGuardedLocked(ctx context.Context, key dnsconfig.AdapterKey) error {
	if err := c.mayApplyLocalDnsLocked(ctx); err != nil {
		return err
	}
	return c.dns.ApplyLocalhost(key)
}

// restoreOwnedLocalhostLocked restores only OWNED / PROBABLE_OWNED provenance.
// UNPROVEN_LOCALHOST is never mutated here — it is recorded for diagnostics /
// explicit emergency-restore --force-unproven-localhost.
func (c *Controller) restoreOwnedLocalhostLocked() []dnsconfig.RestoreResult {
	var out []dnsconfig.RestoreResult
	st, _ := c.cfgStore.Load()
	if st == nil || len(st.Adapters) == 0 {
		return out
	}
	results, err := dnsconfig.ReconcileRecovery(c.dns, st)
	if err != nil {
		c.lastRestoreError = err.Error()
		return results
	}
	out = append(out, results...)
	for _, r := range results {
		if r.Decision == dnsconfig.RestoreApplied {
			c.lastDnsRestore = time.Now().UTC()
		}
		if r.Decision == dnsconfig.RestoreFailed {
			c.lastRestoreError = r.Detail
		}
	}
	if len(st.Adapters) == 0 {
		_ = c.cfgStore.Clear()
	} else {
		_ = c.cfgStore.Save(st)
		c.state = dnsconfig.StateRecoveryRequired
	}
	return out
}

// scanUnprovenLocalhostLocked records UNPROVEN_LOCALHOST adapters without mutating DNS.
func (c *Controller) scanUnprovenLocalhostLocked() {
	adapters, err := c.dns.ListAdapters()
	if err != nil {
		return
	}
	st, _ := c.cfgStore.Load()
	var suspected []string
	for _, a := range adapters {
		snap, err := c.dns.Snapshot(a.Key)
		if err != nil {
			continue
		}
		prov := dnsconfig.ClassifyProvenance(st, a.Key, snap)
		if prov == dnsconfig.ProvenanceUnprovenLocalhost {
			suspected = append(suspected, a.Key.GUID)
		}
	}
	c.suspectedUnproven = suspected
	if len(suspected) == 0 {
		return
	}
	c.lastRecoveryAction = "unproven_localhost_detected"
	// Do not auto-mutate. Elevate diagnostic state when idle.
	if c.state == dnsconfig.StateDisabled || c.state == dnsconfig.StateDegraded {
		c.state = dnsconfig.StateRecoveryRequired
	}
	if st == nil {
		st = &dnsconfig.RecoveryState{
			Version: dnsconfig.RecoveryStateVersion, PolicyVersion: dnsconfig.AdapterPolicyVersion,
			Controller: dnsconfig.StateRecoveryRequired, Dirty: true, SavedAt: time.Now().UTC(),
		}
	}
	st.SuspectedUnprovenLocalhost = append([]string(nil), suspected...)
	st.Controller = dnsconfig.StateRecoveryRequired
	st.Dirty = true
	_ = c.cfgStore.Save(st)
}

func (c *Controller) watchdogParams() (time.Duration, int) {
	every := c.watchdogEvery
	if every == 0 {
		every = watchdogInterval
	}
	limit := c.watchdogFails
	if limit == 0 {
		limit = watchdogFailLimit
	}
	return every, limit
}

func (c *Controller) startWatchdogLocked() {
	if c.watchdogStop != nil {
		return
	}
	stop := make(chan struct{})
	c.watchdogStop = stop
	every, _ := c.watchdogParams()
	go c.watchdogLoop(stop, every)
}

func (c *Controller) stopWatchdogLocked() {
	if c.watchdogStop == nil {
		return
	}
	close(c.watchdogStop)
	c.watchdogStop = nil
	c.healthFailCount = 0
}

func (c *Controller) watchdogLoop(stop <-chan struct{}, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			c.watchdogTick()
		}
	}
}

func (c *Controller) watchdogTick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != dnsconfig.StateActive || c.engine == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := c.mayApplyLocalDnsLocked(ctx)
	if err == nil {
		c.healthFailCount = 0
		c.listenerHealthy = true
		return
	}
	c.healthFailCount++
	c.listenerHealthy = false
	c.lastHealthFailure = err.Error()
	c.lastHealthFailureAt = time.Now().UTC()
	_, limit := c.watchdogParams()
	if c.healthFailCount < limit {
		return
	}
	c.lastRecoveryAction = "watchdog_restore"
	results := c.restoreOwnedLocalhostLocked()
	restoreFailed := false
	for _, r := range results {
		if r.Decision == dnsconfig.RestoreFailed {
			restoreFailed = true
		}
	}
	st, _ := c.cfgStore.Load()
	stillOwned := st != nil && len(st.Adapters) > 0
	c.scanUnprovenLocalhostLocked()

	if c.engine != nil {
		c.engine.Stop()
		c.engine = nil
	}
	c.session = ""
	c.stopWatchdogLocked()

	if restoreFailed || stillOwned {
		c.state = dnsconfig.StateRecoveryRequired
		c.lastErr = "DNS-1 watchdog: listener unhealthy; restore incomplete; provenance retained"
		if c.lastRestoreError == "" {
			c.lastRestoreError = "watchdog restore failed or ownership remains"
		}
		return
	}
	c.state = dnsconfig.StateDegraded
	c.lastErr = "DNS-1 watchdog: listener unhealthy; owned DNS restored; auto-reapply forbidden"
}
