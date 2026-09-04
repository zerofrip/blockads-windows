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

	adapters, err := c.dns.ListAdapters()
	if err != nil {
		engine.Stop()
		c.state = dnsconfig.StateDisabled
		c.lastErr = err.Error()
		return err
	}
	eligible := dnsconfig.FilterEligible(adapters)
	if len(eligible) == 0 {
		engine.Stop()
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
		engine.Stop()
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
		own := dnsconfig.AdapterOwnership{
			Key: a.Key, Original: snap, Applied: dnsconfig.LocalhostApplied, UpdatedAt: time.Now().UTC(),
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
		if err := c.dns.ApplyLocalhost(a.Key); err != nil {
			rollback()
			c.state = dnsconfig.StateDisabled
			c.lastErr = err.Error()
			_ = c.cfgStore.Clear()
			return err
		}
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

	c.engine = engine
	c.checker = checker
	c.session = session
	c.state = dnsconfig.StateActive
	c.appConfig.Enabled = true
	_ = c.appCfg.Save(c.appConfig)
	return nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acceptMut = false
	defer func() { c.acceptMut = true }()
	c.state = dnsconfig.StateStopping

	st, err := c.cfgStore.Load()
	if err != nil {
		c.lastErr = err.Error()
		return err
	}
	if st != nil && len(st.Adapters) > 0 {
		_, err := dnsconfig.ReconcileRecovery(c.dns, st)
		if err != nil {
			c.lastErr = err.Error()
			c.state = dnsconfig.StateDegraded
			_ = c.cfgStore.Save(st)
			return err
		}
		if len(st.Adapters) == 0 {
			_ = c.cfgStore.Clear()
		} else {
			_ = c.cfgStore.Save(st)
		}
	}
	if c.engine != nil {
		c.engine.Stop()
		c.engine = nil
	}
	c.session = ""
	c.appConfig.Enabled = false
	_ = c.appCfg.Save(c.appConfig)
	if st != nil && len(st.Adapters) > 0 {
		c.state = dnsconfig.StateDegraded
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
	st := statusdto.Status{
		Service: statusdto.ServiceStatus{
			Version: AppVersion,
			Uptime:  time.Since(c.startedAt).Truncate(time.Second).String(),
			State:   "running",
			Started: c.startedAt,
		},
		Engine: statusdto.EngineStatus{
			State:            string(c.state),
			ListenerIPv4:     fmt.Sprintf("127.0.0.1:%d", port),
			ListenerIPv6:     fmt.Sprintf("[::1]:%d", port),
			Protocol:         cfg.DNS.Protocol,
			Upstream:         cfg.DNS.Primary,
			LastError:        c.lastErr,
			FilteringEnabled: c.state == dnsconfig.StateActive,
		},
		DNS: statusdto.DNSStatus{
			State:            string(c.state),
			RecoveryRequired: c.state == dnsconfig.StateRecoveryRequired || c.state == dnsconfig.StateDegraded,
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

	rec, _ := c.cfgStore.Load()
	if rec != nil {
		for _, o := range rec.Adapters {
			owned := false
			cat := "unknown"
			conflict := false
			if cur, err := c.dns.Snapshot(o.Key); err == nil {
				if o.MatchesApplied(cur) {
					owned = true
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
				Eligible: true, Owned: owned, StateCategory: cat, RestoreConflict: conflict,
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
func (c *Controller) ReevaluateAdapters(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != dnsconfig.StateActive {
		return nil
	}
	// For Phase 3: re-run enable path pieces for new adapters only
	adapters, err := c.dns.ListAdapters()
	if err != nil {
		return err
	}
	st, err := c.cfgStore.Load()
	if err != nil || st == nil {
		return err
	}
	have := map[string]bool{}
	for _, o := range st.Adapters {
		have[o.Key.GUID] = true
	}
	for _, a := range dnsconfig.FilterEligible(adapters) {
		if have[a.Key.GUID] {
			continue
		}
		snap, err := c.dns.Snapshot(a.Key)
		if err != nil {
			continue
		}
		own := dnsconfig.AdapterOwnership{
			Key: a.Key, Original: snap, Applied: dnsconfig.LocalhostApplied, UpdatedAt: time.Now().UTC(),
		}
		if err := c.dns.ApplyLocalhost(a.Key); err != nil {
			continue
		}
		st.Adapters = append(st.Adapters, own)
		c.names[a.Key.GUID] = a.FriendlyName
	}
	// Drop missing
	present := map[string]bool{}
	for _, a := range adapters {
		present[a.Key.GUID] = true
	}
	kept := st.Adapters[:0]
	for _, o := range st.Adapters {
		if present[o.Key.GUID] {
			kept = append(kept, o)
		}
	}
	st.Adapters = kept
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

