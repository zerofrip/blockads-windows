package statusdto

import "time"

// Status is the primary diagnostic contract for CLI/future UI (no query log).
type Status struct {
	Service ServiceStatus `json:"service"`
	Engine  EngineStatus  `json:"engine"`
	DNS     DNSStatus     `json:"dns"`
	Filters FilterStatus  `json:"filters"`
	Stats   StatsStatus   `json:"stats"`
}

type ServiceStatus struct {
	Version string    `json:"version"`
	Uptime  string    `json:"uptime"`
	State   string    `json:"state"` // installed/running host state vs filtering
	Started time.Time `json:"startedAt,omitempty"`
}

type EngineStatus struct {
	State             string `json:"state"`
	ListenerIPv4      string `json:"listenerIPv4"`
	ListenerIPv6      string `json:"listenerIPv6"`
	Protocol          string `json:"protocol"`
	Upstream          string `json:"upstream"`
	DoHURL            string `json:"dohUrl,omitempty"`
	LastError         string `json:"lastError,omitempty"`
	FilteringEnabled  bool   `json:"filteringEnabled"`
	ListenerHealthy   bool   `json:"listenerHealthy"`
	NetwatchCallbacks uint64 `json:"netwatchCallbacks,omitempty"`
	NetwatchReevals   uint64 `json:"netwatchReevaluations,omitempty"`
}

// DNSStatus separates desired protection, runtime engine, and DNS ownership (DNS-1/DNS-2).
type DNSStatus struct {
	State                      string          `json:"state"`
	DesiredProtection          string          `json:"desiredProtection"`
	RuntimeProtection          string          `json:"runtimeProtection"`
	DnsOwnership               string          `json:"dnsOwnership"`
	ListenerHealth             bool            `json:"listenerHealth"`
	RecoveryRequired           bool            `json:"recoveryRequired"`
	OwnedAdapterCount          int             `json:"ownedAdapterCount,omitempty"`
	SuspectedUnprovenLocalhost []string        `json:"suspectedUnprovenLocalhost,omitempty"`
	LastHealthFailure          string          `json:"lastHealthFailure,omitempty"`
	LastHealthFailureAt        time.Time       `json:"lastHealthFailureAt,omitempty"`
	LastDnsApply               time.Time       `json:"lastDnsApply,omitempty"`
	LastDnsRestore             time.Time       `json:"lastDnsRestore,omitempty"`
	LastNetworkChange          time.Time       `json:"lastNetworkChange,omitempty"`
	LastRecoveryAction         string          `json:"lastRecoveryAction,omitempty"`
	LastRestoreError           string          `json:"lastRestoreError,omitempty"`
	Adapters                   []AdapterStatus `json:"adapters"`
}

type AdapterStatus struct {
	StableID        string `json:"stableId"`
	DisplayName     string `json:"displayName"`
	Eligible        bool   `json:"eligible"`
	Owned           bool   `json:"owned"`
	StateCategory   string `json:"currentStateCategory"` // original|blockads|conflict|unknown|missing
	RestoreConflict bool   `json:"restoreConflict"`
}

type FilterStatus struct {
	Loaded          bool      `json:"loaded"`
	ListIDs         []string  `json:"listIds"`
	LastUpdate      time.Time `json:"lastUpdate,omitempty"`
	LastUpdateError string    `json:"lastUpdateError,omitempty"`
}

type StatsStatus struct {
	TotalQueries   int64 `json:"totalQueries"`
	BlockedQueries int64 `json:"blockedQueries"`
}
