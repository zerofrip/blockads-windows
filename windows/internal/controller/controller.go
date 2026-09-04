package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	tunnel "github.com/nqmgaming/blockads-tunnel"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

// Paths holds on-disk locations for the Windows MVP.
type Paths struct {
	StateFile   string
	FilterDir   string
	DataDir     string
}

// Config is runtime configuration for enable.
type Config struct {
	ListenPort   int
	DNSProtocol  string // udp, doh, …
	PrimaryDNS   string
	FallbackDNS  string
	DoHURL       string
	AdTrieCSV    string
	AdBloomCSV   string
	SecTrieCSV   string
	SecBloomCSV  string
	AllowRules   []string
	BlockRules   []string
	Whitelist    []string
}

// StatusSnapshot is safe for CLI display (no secrets).
type StatusSnapshot struct {
	State           dnsconfig.EngineState `json:"state"`
	ListenerOK      bool                  `json:"listenerOk"`
	ListenAddr      string                `json:"listenAddr"`
	SessionID       string                `json:"sessionId,omitempty"`
	Selected        []string              `json:"selectedAdapters"`
	Ownership       []OwnershipView       `json:"ownership"`
	RecoveryDirty   bool                  `json:"recoveryDirty"`
	Upstream        string                `json:"upstream"`
	FilterLoaded    bool                  `json:"filterLoaded"`
	LastError       string                `json:"lastError,omitempty"`
	PortConflict    string                `json:"portConflict,omitempty"`
}

type OwnershipView struct {
	GUID     string `json:"guid"`
	Name     string `json:"name,omitempty"`
	Owned    bool   `json:"owned"`
	IPv4     string `json:"ipv4Applied"`
}

// Controller owns transactional enable/disable and recovery.
type Controller struct {
	cfgStore *dnsconfig.StateStore
	dns      dnsconfig.DnsConfigurator
	mu       sync.Mutex
	state    dnsconfig.EngineState
	engine   *tunnel.Engine
	checker  *tunnel.CustomRuleChecker
	session  string
	lastErr  string
	portInfo string
	paths    Paths
	config   Config
	names    map[string]string // guid → friendly
}

func New(paths Paths, dnsCfg dnsconfig.DnsConfigurator) *Controller {
	return &Controller{
		cfgStore: dnsconfig.NewStateStore(paths.StateFile),
		dns:      dnsCfg,
		state:    dnsconfig.StateDisabled,
		paths:    paths,
		names:    make(map[string]string),
		config: Config{
			ListenPort:  53,
			DNSProtocol: "udp",
			PrimaryDNS:  "1.1.1.1",
			FallbackDNS: "1.0.0.1",
		},
	}
}

func (c *Controller) SetConfig(cfg Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 53
	}
	c.config = cfg
}

func newSessionID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// RecoverIfNeeded loads recovery state and reconciles owned DNS.
func (c *Controller) RecoverIfNeeded() ([]dnsconfig.RestoreResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
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

// Enable starts the listener then applies DNS transactionally.
func (c *Controller) Enable(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == dnsconfig.StateActive {
		return nil
	}
	c.state = dnsconfig.StateStarting
	c.lastErr = ""
	c.portInfo = ""

	port := c.config.ListenPort
	if err := checkPortAvailable(port); err != nil {
		c.state = dnsconfig.StateDisabled
		c.lastErr = err.Error()
		c.portInfo = err.Error()
		return err
	}

	engine := tunnel.NewEngine()
	checker := tunnel.NewCustomRuleChecker()
	checker.SetRules(c.config.AllowRules, c.config.BlockRules, c.config.Whitelist)
	engine.SetDomainChecker(checker)
	engine.SetDNS(c.config.DNSProtocol, c.config.PrimaryDNS, c.config.FallbackDNS, c.config.DoHURL)
	if c.config.AdTrieCSV != "" || c.config.SecTrieCSV != "" {
		engine.SetTries(c.config.AdTrieCSV, c.config.SecTrieCSV, c.config.AdBloomCSV, c.config.SecBloomCSV)
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
	appliedKeys := make([]dnsconfig.AdapterKey, 0)

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
			Key:       a.Key,
			Original:  snap,
			Applied:   dnsconfig.LocalhostApplied,
			UpdatedAt: time.Now().UTC(),
		}
		// Persist recovery BEFORE mutating DNS
		st := &dnsconfig.RecoveryState{
			Version:       dnsconfig.RecoveryStateVersion,
			PolicyVersion: dnsconfig.AdapterPolicyVersion,
			SessionID:     session,
			Controller:    dnsconfig.StateStarting,
			ListenAddr:    "127.0.0.1",
			ListenPort:    port,
			Adapters:      append(append([]dnsconfig.AdapterOwnership{}, ownership...), own),
			SavedAt:       time.Now().UTC(),
			Dirty:         true,
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
		appliedKeys = append(appliedKeys, a.Key)
	}

	st := &dnsconfig.RecoveryState{
		Version:       dnsconfig.RecoveryStateVersion,
		PolicyVersion: dnsconfig.AdapterPolicyVersion,
		SessionID:     session,
		Controller:    dnsconfig.StateActive,
		ListenAddr:    "127.0.0.1",
		ListenPort:    port,
		Adapters:      ownership,
		SavedAt:       time.Now().UTC(),
		Dirty:         true,
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
	_ = appliedKeys
	return nil
}

// Disable compare-and-restores owned adapters then stops the listener.
func (c *Controller) Disable() error {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	if st != nil && len(st.Adapters) > 0 {
		c.state = dnsconfig.StateDegraded
	} else {
		c.state = dnsconfig.StateDisabled
	}
	return nil
}

func (c *Controller) Status() StatusSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := StatusSnapshot{
		State:      c.state,
		ListenerOK: c.engine != nil && c.engine.IsRunning(),
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", c.config.ListenPort),
		SessionID:  c.session,
		Upstream:   fmt.Sprintf("%s %s", c.config.DNSProtocol, c.config.PrimaryDNS),
		LastError:  c.lastErr,
		PortConflict: c.portInfo,
		FilterLoaded: c.config.AdTrieCSV != "" || c.config.SecTrieCSV != "",
	}
	st, _ := c.cfgStore.Load()
	if st != nil {
		s.RecoveryDirty = st.Dirty
		for _, o := range st.Adapters {
			cur, err := c.dns.Snapshot(o.Key)
			owned := err == nil && o.MatchesApplied(cur)
			s.Ownership = append(s.Ownership, OwnershipView{
				GUID:  o.Key.GUID,
				Name:  c.names[o.Key.GUID],
				Owned: owned,
				IPv4:  stringsJoin(o.Applied.IPv4Servers),
			})
			s.Selected = append(s.Selected, o.Key.GUID)
		}
	}
	return s
}

func stringsJoin(s []string) string {
	return strings.Join(s, ",")
}

func checkPortAvailable(port int) error {
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("%w: udp4 :%d (%v) — another DNS service may be bound; BlockAds will not displace it",
				dnsconfig.ErrPortInUse, port, err)
		}
		return err
	}
	_ = udp.Close()

	tcp, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("%w: tcp4 :%d (%v) — another DNS service may be bound; BlockAds will not displace it",
				dnsconfig.ErrPortInUse, port, err)
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
		strings.Contains(msg, "only one usage of each socket address") ||
		strings.Contains(msg, "bind: address already in use")
}

func healthCheckLocalDNS(ctx context.Context, port int) error {
	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("example.com"), dns.TypeA)
	_, _, err := client.ExchangeContext(ctx, m, addr)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "connection refused") || strings.Contains(msg, "i/o timeout") {
			return err
		}
	}
	return nil
}
