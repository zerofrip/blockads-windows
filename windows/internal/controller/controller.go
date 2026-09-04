package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	tunnel "github.com/nqmgaming/blockads-tunnel"
	"github.com/nqmgaming/blockads-windows/windows/internal/config"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
	"github.com/nqmgaming/blockads-windows/windows/internal/filters"
	"github.com/nqmgaming/blockads-windows/windows/internal/netwatch"
	"github.com/nqmgaming/blockads-windows/windows/internal/protocol"
	"github.com/nqmgaming/blockads-windows/windows/internal/singleinstance"
	"github.com/nqmgaming/blockads-windows/windows/internal/statusdto"
)

const AppVersion = "0.1.0-windows"

// Paths holds on-disk locations for the Windows service.
type Paths struct {
	DataDir    string
	StateFile  string
	ConfigFile string
	FilterDir  string
}

// Controller is the single production owner of DNS filtering lifecycle.
type Controller struct {
	cfgStore *dnsconfig.StateStore
	appCfg   *config.Store
	dns      dnsconfig.DnsConfigurator
	filters  *filters.Manager
	lock     *singleinstance.Lock

	mu        sync.Mutex
	state     dnsconfig.EngineState
	engine    *tunnel.Engine
	checker   *tunnel.CustomRuleChecker
	session   string
	lastErr   string
	portInfo  string
	paths     Paths
	appConfig config.Config
	names     map[string]string
	listIDs   []string
	startedAt time.Time
	filterErr string
	filterAt  time.Time
	acceptMut bool // false while stopping

	// DNS-1 safety instrumentation (see safety.go).
	healthProbe         HealthProbe
	watchdogStop        chan struct{}
	healthFailCount     int
	listenerHealthy     bool
	lastHealthFailure   string
	lastHealthFailureAt time.Time
	lastRecoveryAction  string
	lastDnsApply        time.Time
	lastDnsRestore      time.Time
	lastNetworkChange   time.Time
	lastRestoreError    string
	suspectedUnproven   []string
	ownershipGeneration int64
	watchdogEvery       time.Duration // 0 → default
	watchdogFails       int           // 0 → default
	testBypassEngine    bool          // tests only; never set in production
}

func New(paths Paths, dnsCfg dnsconfig.DnsConfigurator) (*Controller, error) {
	lock, err := singleinstance.Acquire(paths.DataDir)
	if err != nil {
		return nil, err
	}
	c := &Controller{
		cfgStore:  dnsconfig.NewStateStore(paths.StateFile),
		appCfg:    &config.Store{Path: paths.ConfigFile},
		dns:       dnsCfg,
		filters:   filters.NewManager(paths.FilterDir),
		lock:      lock,
		state:     dnsconfig.StateDisabled,
		paths:     paths,
		names:     make(map[string]string),
		appConfig: config.Default(),
		acceptMut: true,
		startedAt: time.Now().UTC(),
	}
	if cfg, err := c.appCfg.Load(); err == nil {
		c.appConfig = cfg
	}
	return c, nil
}

func (c *Controller) Close() {
	c.lock.Release()
}

// Startup runs ownership-safe Recover, scans unproven localhost (no auto-mutate),
// restores only proven ownership leftovers, then applies desired protection.
func (c *Controller) Startup(ctx context.Context) error {
	_, recErr := c.Recover(ctx)
	c.mu.Lock()
	// Proven ownership only — never auto-reset UNPROVEN_LOCALHOST.
	_ = c.restoreOwnedLocalhostLocked()
	c.scanUnprovenLocalhostLocked()
	want := c.appConfig.Enabled
	c.mu.Unlock()
	if !want {
		return recErr
	}
	if err := c.Enable(ctx); err != nil {
		c.mu.Lock()
		_ = c.restoreOwnedLocalhostLocked()
		c.scanUnprovenLocalhostLocked()
		c.mu.Unlock()
		if recErr != nil {
			return fmt.Errorf("recover: %v; enable desired: %w", recErr, err)
		}
		return err
	}
	return nil
}

func newSessionID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (c *Controller) Recover(ctx context.Context) ([]dnsconfig.RestoreResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recoverLocked()
}

func (c *Controller) recoverLocked() ([]dnsconfig.RestoreResult, error) {
	st, err := c.cfgStore.Load()
	if err != nil {
		return nil, err
	}
	if st == nil || (!st.Dirty && len(st.Adapters) == 0) {
		return nil, nil
	}
	c.state = dnsconfig.StateRecoveryRequired
	results, err := dnsconfig.ReconcileRecovery(c.dns, st)
	if err != nil {
		c.lastErr = err.Error()
		return results, err
	}
	if err := c.cfgStore.Save(st); err != nil {
		return results, err
	}
	if !st.Dirty {
		_ = c.cfgStore.Clear()
		c.state = dnsconfig.StateDisabled
	} else {
		c.state = st.Controller
	}
	return results, nil
}

func (c *Controller) Enable(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.acceptMut {
		return errors.New("service is stopping")
	}
	if c.state == dnsconfig.StateActive {
		return nil
	}
	if cfg, err := c.appCfg.Load(); err == nil {
		c.appConfig = cfg
	}
	c.state = dnsconfig.StateStarting
	c.lastErr = ""
	c.portInfo = ""

	cfg := c.appConfig
	port := cfg.DNS.ListenPort
	if port == 0 {
		port = 53
	}
	if err := checkPortAvailable(port); err != nil {
		c.state = dnsconfig.StateDisabled
		c.lastErr = err.Error()
		c.portInfo = err.Error()
		return err
	}

	engine := tunnel.NewEngine()
	checker := tunnel.NewCustomRuleChecker()
	engine.SetDomainChecker(checker)
	engine.SetDNS(cfg.DNS.Protocol, cfg.DNS.Primary, cfg.DNS.Fallback, cfg.DNS.DoHURL)

	// Load filters if present
	if err := c.loadFiltersLocked(ctx, engine); err != nil {
		c.filterErr = err.Error()
		// Continue without filters only if none configured; otherwise fail
		if len(cfg.Filters.EnabledListIDs) > 0 || cfg.Filters.AutoUpdate {
			// try continue with empty if download failed at first run — still start DNS engine
		}
	}

	if err := engine.StartStandalone(port); err != nil {
		c.state = dnsconfig.StateDisabled
		c.lastErr = err.Error()
		if isAddrInUse(err) {
			c.portInfo = err.Error()
			return fmt.Errorf("%w: %v", dnsconfig.ErrPortInUse, err)
		}
		return err
	}
	if err := healthCheckLocalDNS(ctx, port); err != nil {
		engine.Stop()
		c.state = dnsconfig.StateDisabled
		c.lastErr = err.Error()
		return fmt.Errorf("%w: %v", dnsconfig.ErrListenerUnhealthy, err)
	}

	// Assign engine before any DNS apply so mayApplyLocalDns can authorize.
	c.engine = engine
	c.checker = checker
	c.listenerHealthy = true

	adapters, err := c.dns.ListAdapters()
	if err != nil {
		engine.Stop()
		c.engine = nil
		c.state = dnsconfig.StateDisabled
		c.lastErr = err.Error()
		return err
	}
	eligible := dnsconfig.FilterEligible(adapters)
	if len(eligible) == 0 {
		engine.Stop()
		c.engine = nil
		c.state = dnsconfig.StateDisabled
		c.lastErr = "no eligible adapters"
		return fmt.Errorf("no eligible network adapters for DNS configuration")
	}

	session := newSessionID()
	ownership := make([]dnsconfig.AdapterOwnership, 0, len(eligible))

	rollback := func() {
		for i := len(ownership) - 1; i >= 0; i-- {
			own := ownership[i]
			cur, err := c.dns.Snapshot(own.Key)
			if err != nil {
				continue
			}
			_ = dnsconfig.CompareAndRestore(c.dns, own, cur, true)
		}
		c.stopWatchdogLocked()
		engine.Stop()
		c.engine = nil
	}

	for _, a := range eligible {
		c.names[a.Key.GUID] = a.FriendlyName
		snap, err := c.dns.Snapshot(a.Key)
		if err != nil {
			rollback()
			c.state = dnsconfig.StateDisabled
			c.lastErr = err.Error()
			return err
		}
		snap.FriendlyName = a.FriendlyName
		if !dnsconfig.CanBecomeOriginal(snap) {
			// SUSPECT_LOCALHOST_ORPHAN — do not invent Original=localhost; do not blind-reset.
			c.scanUnprovenLocalhostLocked()
			rollback()
			c.state = dnsconfig.StateRecoveryRequired
			c.lastErr = fmt.Sprintf("adapter %s has localhost DNS without trustworthy Original (class=%s); use emergency-restore for proven ownership or --force-unproven-localhost",
				a.Key.GUID, dnsconfig.ClassifyDNS(snap))
			return fmt.Errorf("%s", c.lastErr)
		}
		c.ownershipGeneration++
		own, err := dnsconfig.BeginOwnership(a.Key, snap, session, c.ownershipGeneration)
		if err != nil {
			rollback()
			c.state = dnsconfig.StateDisabled
			c.lastErr = err.Error()
			return err
		}
		st := &dnsconfig.RecoveryState{
			Version: dnsconfig.RecoveryStateVersion, PolicyVersion: dnsconfig.AdapterPolicyVersion,
			SessionID: session, Controller: dnsconfig.StateStarting,
			ListenAddr: "127.0.0.1", ListenPort: port,
			Adapters: append(append([]dnsconfig.AdapterOwnership{}, ownership...), own),
			SavedAt: time.Now().UTC(), Dirty: true,
		}
		if err := c.cfgStore.Save(st); err != nil {
			rollback()
			c.state = dnsconfig.StateDisabled
			c.lastErr = err.Error()
			return err
		}
		if err := c.applyLocalhostGuardedLocked(ctx, a.Key); err != nil {
			rollback()
			c.state = dnsconfig.StateDisabled
			c.lastErr = err.Error()
			_ = c.cfgStore.Clear()
			return err
		}
		if err := own.MarkOwned(); err != nil {
			rollback()
			c.state = dnsconfig.StateDisabled
			c.lastErr = err.Error()
			return err
		}
		c.lastDnsApply = time.Now().UTC()
		ownership = append(ownership, own)
	}

	st := &dnsconfig.RecoveryState{
		Version: dnsconfig.RecoveryStateVersion, PolicyVersion: dnsconfig.AdapterPolicyVersion,
		SessionID: session, Controller: dnsconfig.StateActive,
		ListenAddr: "127.0.0.1", ListenPort: port,
		Adapters: ownership, SavedAt: time.Now().UTC(), Dirty: true,
	}
	if err := c.cfgStore.Save(st); err != nil {
		rollback()
		c.state = dnsconfig.StateDisabled
		c.lastErr = err.Error()
		return err
	}

	c.session = session
	c.state = dnsconfig.StateActive
	c.appConfig.Enabled = true
	_ = c.persistEnabledLocked(true)
	c.startWatchdogLocked()
	return nil
}

func (c *Controller) persistEnabledLocked(enabled bool) error {
	cfg := c.appConfig
	if disk, err := c.appCfg.Load(); err == nil {
		cfg = disk
	}
	cfg.Enabled = enabled
	c.appConfig = cfg
	return c.appCfg.Save(cfg)
}

func (c *Controller) loadFiltersLocked(ctx context.Context, engine *tunnel.Engine) error {
	cfg := c.appConfig
	entries, err := c.filters.FetchCatalog(ctx, cfg.Filters.CatalogURL)
	if err != nil {
		// try local catalog / current lists
		paths, _ := c.filters.PathsFromCurrent(nil, cfg.Filters.EnabledListIDs)
		if paths.AdTrieCSV == "" && paths.SecTrieCSV == "" {
			return err
		}
		return engine.ReplaceTriesAtomic(paths.AdTrieCSV, paths.SecTrieCSV, paths.AdBloomCSV, paths.SecBloomCSV)
	}
	prepared, err := c.filters.PrepareVersioned(ctx, entries, cfg.Filters.EnabledListIDs)
	if err != nil {
		return err
	}
	if prepared.AdTrieCSV == "" && prepared.SecTrieCSV == "" {
		return nil
	}
	if err := engine.ReplaceTriesAtomic(prepared.AdTrieCSV, prepared.SecTrieCSV, prepared.AdBloomCSV, prepared.SecBloomCSV); err != nil {
		c.filters.AbortPrepared(prepared)
		return err
	}
	c.filters.CommitPrepared(prepared)
	c.listIDs = prepared.ListIDs
	c.filterAt = time.Now().UTC()
	c.filterErr = ""
	return nil
}

func (c *Controller) Disable(ctx context.Context) error {
	return c.stopRuntime(ctx, true, "disable_restore")
}

// ShutdownForServiceStop restores DNS and stops the listener without clearing
// desired protection. SCM Stop/Shutdown must not persist enabled=false so that
// Startup can re-apply filtering after reboot or service restart.
func (c *Controller) ShutdownForServiceStop(ctx context.Context) error {
	return c.stopRuntime(ctx, false, "service_stop_restore")
}

func (c *Controller) stopRuntime(ctx context.Context, clearDesired bool, recoveryAction string) error {
	_ = ctx
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acceptMut = false
	defer func() { c.acceptMut = true }()
	c.state = dnsconfig.StateStopping
	c.stopWatchdogLocked()

	results := c.restoreOwnedLocalhostLocked()
	for _, r := range results {
		if r.Decision == dnsconfig.RestoreApplied {
			c.lastRecoveryAction = recoveryAction
		}
	}
	c.scanUnprovenLocalhostLocked()

	if c.engine != nil {
		c.engine.Stop()
		c.engine = nil
	}
	c.session = ""
	c.listenerHealthy = false
	if clearDesired {
		_ = c.persistEnabledLocked(false)
	}
	st, _ := c.cfgStore.Load()
	if st != nil && (len(st.Adapters) > 0 || len(st.SuspectedUnprovenLocalhost) > 0) {
		c.state = dnsconfig.StateRecoveryRequired
	} else if len(c.suspectedUnproven) > 0 {
		c.state = dnsconfig.StateRecoveryRequired
	} else {
		c.state = dnsconfig.StateDisabled
	}
	return nil
}

func (c *Controller) ReloadFilters(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.engine == nil {
		return fmt.Errorf("%s: engine not running", protocol.CodeEngineError)
	}
	if cfg, err := c.appCfg.Load(); err == nil {
		c.appConfig = cfg
	}
	if err := c.loadFiltersLocked(ctx, c.engine); err != nil {
		c.filterErr = err.Error()
		return err
	}
	return nil
}

func (c *Controller) TestDNS(ctx context.Context, domain string) (protocol.TestDNSResult, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return protocol.TestDNSResult{}, fmt.Errorf("domain required")
	}
	c.mu.Lock()
	port := c.appConfig.DNS.ListenPort
	if port == 0 {
		port = 53
	}
	running := c.engine != nil && c.engine.IsRunning()
	c.mu.Unlock()
	if !running {
		return protocol.TestDNSResult{}, fmt.Errorf("%s: listener not active", protocol.CodeEngineError)
	}
	client := &dns.Client{Net: "udp", Timeout: 5 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	r, _, err := client.ExchangeContext(ctx, m, fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return protocol.TestDNSResult{}, err
	}
	out := protocol.TestDNSResult{
		Domain:  domain,
		RCode:   dns.RcodeToString[r.Rcode],
		Blocked: r.Rcode == dns.RcodeNameError || r.Rcode == dns.RcodeRefused || r.Rcode == dns.RcodeSuccess && len(r.Answer) == 0,
	}
	for _, rr := range r.Answer {
		out.Answers = append(out.Answers, rr.String())
	}
	// refine blocked: NXDOMAIN / 0.0.0.0 style
	if r.Rcode == dns.RcodeNameError {
		out.Blocked = true
	}
	return out, nil
}

func (c *Controller) GetStats() protocol.StatsResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.engine == nil {
		return protocol.StatsResult{}
	}
	var s struct {
		TotalQueries   int64 `json:"TotalQueries"`
		BlockedQueries int64 `json:"BlockedQueries"`
	}
	_ = json.Unmarshal([]byte(c.engine.GetStats()), &s)
	return protocol.StatsResult{TotalQueries: s.TotalQueries, BlockedQueries: s.BlockedQueries}
}

func (c *Controller) Status() statusdto.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg := c.appConfig
	port := cfg.DNS.ListenPort
	if port == 0 {
		port = 53
	}
	nwCB, nwRe := netwatch.Stats()
	desired := "DISABLED"
	if cfg.Enabled {
		desired = "ENABLED"
	}
	runtime := string(c.state)
	ownership := "NONE"
	rec, _ := c.cfgStore.Load()
	owned := 0
	if rec != nil {
		owned = len(rec.Adapters)
		if owned > 0 {
			ownership = "OWNED"
		}
	}
	st := statusdto.Status{
		Service: statusdto.ServiceStatus{
			Version: AppVersion,
			Uptime:  time.Since(c.startedAt).Truncate(time.Second).String(),
			State:   "running",
			Started: c.startedAt,
		},
		Engine: statusdto.EngineStatus{
			State:             runtime,
			ListenerIPv4:      fmt.Sprintf("127.0.0.1:%d", port),
			ListenerIPv6:      fmt.Sprintf("[::1]:%d", port),
			Protocol:          cfg.DNS.Protocol,
			Upstream:          cfg.DNS.Primary,
			DoHURL:            cfg.DNS.DoHURL,
			LastError:         c.lastErr,
			FilteringEnabled:  c.state == dnsconfig.StateActive,
			ListenerHealthy:   c.listenerHealthy && c.state == dnsconfig.StateActive,
			NetwatchCallbacks: nwCB,
			NetwatchReevals:   nwRe,
		},
		DNS: statusdto.DNSStatus{
			State:                      runtime,
			DesiredProtection:          desired,
			RuntimeProtection:          runtime,
			DnsOwnership:               ownership,
			ListenerHealth:             c.listenerHealthy,
			RecoveryRequired:           c.state == dnsconfig.StateRecoveryRequired || c.state == dnsconfig.StateDegraded || owned > 0 && c.state != dnsconfig.StateActive || len(c.suspectedUnproven) > 0,
			OwnedAdapterCount:          owned,
			SuspectedUnprovenLocalhost: append([]string(nil), c.suspectedUnproven...),
			LastHealthFailure:          c.lastHealthFailure,
			LastHealthFailureAt:        c.lastHealthFailureAt,
			LastDnsApply:               c.lastDnsApply,
			LastDnsRestore:             c.lastDnsRestore,
			LastNetworkChange:          c.lastNetworkChange,
			LastRecoveryAction:         c.lastRecoveryAction,
			LastRestoreError:           c.lastRestoreError,
		},
		Filters: statusdto.FilterStatus{
			Loaded:          len(c.listIDs) > 0,
			ListIDs:         append([]string(nil), c.listIDs...),
			LastUpdate:      c.filterAt,
			LastUpdateError: c.filterErr,
		},
	}
	stats := c.GetStatsUnlocked()
	st.Stats = statusdto.StatsStatus{TotalQueries: stats.TotalQueries, BlockedQueries: stats.BlockedQueries}

	if rec != nil {
		for _, o := range rec.Adapters {
			ownedFlag := false
			cat := "unknown"
			conflict := false
			if cur, err := c.dns.Snapshot(o.Key); err == nil {
				if o.MatchesApplied(cur) {
					ownedFlag = true
					cat = "blockads"
				} else {
					conflict = true
					cat = "conflict"
				}
			} else {
				cat = "missing"
			}
			st.DNS.Adapters = append(st.DNS.Adapters, statusdto.AdapterStatus{
				StableID: o.Key.GUID, DisplayName: c.names[o.Key.GUID],
				Eligible: true, Owned: ownedFlag, StateCategory: cat, RestoreConflict: conflict,
			})
		}
	}
	return st
}

func (c *Controller) GetStatsUnlocked() protocol.StatsResult {
	if c.engine == nil {
		return protocol.StatsResult{}
	}
	var s struct {
		TotalQueries   int64 `json:"TotalQueries"`
		BlockedQueries int64 `json:"BlockedQueries"`
	}
	_ = json.Unmarshal([]byte(c.engine.GetStats()), &s)
	return protocol.StatsResult{TotalQueries: s.TotalQueries, BlockedQueries: s.BlockedQueries}
}

// ReevaluateAdapters applies ownership policy after network changes.
//
// DNS-1/DNS-2: never ApplyLocalhost unless mayApplyLocalDns authorizes.
// Never re-snapshot Original. Never drop ownership on temporary absence.
// Reappearance reconciles against the existing ownership record only.
func (c *Controller) ReevaluateAdapters(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastNetworkChange = time.Now().UTC()
	if c.state != dnsconfig.StateActive || !c.acceptMut {
		return nil
	}
	if err := c.mayApplyLocalDnsLocked(ctx); err != nil {
		return nil
	}
	adapters, err := c.dns.ListAdapters()
	if err != nil {
		return err
	}
	st, err := c.cfgStore.Load()
	if err != nil || st == nil {
		return err
	}
	have := map[string]bool{}
	for i := range st.Adapters {
		have[st.Adapters[i].Key.GUID] = true
		st.Adapters[i].LastConfirmedAt = time.Now().UTC()
		st.Adapters[i].UpdatedAt = st.Adapters[i].LastConfirmedAt
	}
	for _, a := range dnsconfig.FilterEligible(adapters) {
		if have[a.Key.GUID] {
			continue
		}
		snap, err := c.dns.Snapshot(a.Key)
		if err != nil {
			continue
		}
		if !dnsconfig.CanBecomeOriginal(snap) {
			continue
		}
		c.ownershipGeneration++
		own, err := dnsconfig.BeginOwnership(a.Key, snap, c.session, c.ownershipGeneration)
		if err != nil {
			continue
		}
		if err := c.applyLocalhostGuardedLocked(ctx, a.Key); err != nil {
			continue
		}
		_ = own.MarkOwned()
		c.lastDnsApply = time.Now().UTC()
		st.Adapters = append(st.Adapters, own)
		c.names[a.Key.GUID] = a.FriendlyName
	}
	st.SavedAt = time.Now().UTC()
	return c.cfgStore.Save(st)
}

func checkPortAvailable(port int) error {
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("%w: udp4 :%d (%v)", dnsconfig.ErrPortInUse, port, err)
		}
		return err
	}
	_ = udp.Close()
	tcp, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("%w: tcp4 :%d (%v)", dnsconfig.ErrPortInUse, port, err)
		}
		return err
	}
	_ = tcp.Close()
	return nil
}

func isAddrInUse(err error) bool {
	var op *net.OpError
	if errors.As(err, &op) {
		err = op.Err
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}

func healthCheckLocalDNS(ctx context.Context, port int) error {
	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("example.com"), dns.TypeA)
	_, _, err := client.ExchangeContext(ctx, m, fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "connection refused") || strings.Contains(msg, "i/o timeout") {
			return err
		}
	}
	return nil
}



