using System.Text.Json.Serialization;

namespace BlockAds.Windows.Models;

public sealed class ServiceStatusDto
{
    [JsonPropertyName("version")] public string Version { get; set; } = "";
    [JsonPropertyName("uptime")] public string Uptime { get; set; } = "";
    [JsonPropertyName("state")] public string State { get; set; } = "";
    [JsonPropertyName("startedAt")] public DateTimeOffset? StartedAt { get; set; }
}

public sealed class EngineStatusDto
{
    [JsonPropertyName("state")] public string State { get; set; } = "";
    [JsonPropertyName("listenerIPv4")] public string ListenerIPv4 { get; set; } = "";
    [JsonPropertyName("listenerIPv6")] public string ListenerIPv6 { get; set; } = "";
    [JsonPropertyName("protocol")] public string Protocol { get; set; } = "";
    [JsonPropertyName("upstream")] public string Upstream { get; set; } = "";
    [JsonPropertyName("dohUrl")] public string? DohUrl { get; set; }
    [JsonPropertyName("lastError")] public string? LastError { get; set; }
    [JsonPropertyName("filteringEnabled")] public bool FilteringEnabled { get; set; }
    [JsonPropertyName("listenerHealthy")] public bool ListenerHealthy { get; set; }
}

public sealed class DnsStatusDto
{
    [JsonPropertyName("state")] public string State { get; set; } = "";
    [JsonPropertyName("desiredProtection")] public string DesiredProtection { get; set; } = "";
    [JsonPropertyName("runtimeProtection")] public string RuntimeProtection { get; set; } = "";
    [JsonPropertyName("dnsOwnership")] public string DnsOwnership { get; set; } = "";
    [JsonPropertyName("listenerHealth")] public bool ListenerHealth { get; set; }
    [JsonPropertyName("recoveryRequired")] public bool RecoveryRequired { get; set; }
    [JsonPropertyName("ownedAdapterCount")] public int OwnedAdapterCount { get; set; }
    [JsonPropertyName("lastHealthFailure")] public string? LastHealthFailure { get; set; }
    [JsonPropertyName("lastHealthFailureAt")] public DateTimeOffset? LastHealthFailureAt { get; set; }
    [JsonPropertyName("lastDnsApply")] public DateTimeOffset? LastDnsApply { get; set; }
    [JsonPropertyName("lastDnsRestore")] public DateTimeOffset? LastDnsRestore { get; set; }
    [JsonPropertyName("lastNetworkChange")] public DateTimeOffset? LastNetworkChange { get; set; }
    [JsonPropertyName("lastRecoveryAction")] public string? LastRecoveryAction { get; set; }
    [JsonPropertyName("lastRestoreError")] public string? LastRestoreError { get; set; }
}

public sealed class FilterStatusDto
{
    [JsonPropertyName("loaded")] public bool Loaded { get; set; }
    [JsonPropertyName("listIds")] public List<string>? ListIds { get; set; }
    [JsonPropertyName("lastUpdate")] public DateTimeOffset? LastUpdate { get; set; }
    [JsonPropertyName("lastUpdateError")] public string? LastUpdateError { get; set; }
}

public sealed class StatsStatusDto
{
    [JsonPropertyName("totalQueries")] public long TotalQueries { get; set; }
    [JsonPropertyName("blockedQueries")] public long BlockedQueries { get; set; }
}

public sealed class ServiceStatusSnapshot
{
    [JsonPropertyName("service")] public ServiceStatusDto Service { get; set; } = new();
    [JsonPropertyName("engine")] public EngineStatusDto Engine { get; set; } = new();
    [JsonPropertyName("dns")] public DnsStatusDto Dns { get; set; } = new();
    [JsonPropertyName("filters")] public FilterStatusDto Filters { get; set; } = new();
    [JsonPropertyName("stats")] public StatsStatusDto Stats { get; set; } = new();
}

public enum ProtectionUiState
{
    Off,
    Starting,
    Active,
    Degraded,
    RecoveryRequired,
    Stopping,
    ServiceUnavailable
}

public enum UiErrorCategory
{
    None,
    ServiceUnavailable,
    Timeout,
    ProtocolError,
    EnableFailed,
    DisableFailed,
    RecoveryRequired,
    FilterReloadFailed,
    Internal
}
