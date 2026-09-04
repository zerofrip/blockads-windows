using BlockAds.Windows.Models;

namespace BlockAds.Windows.Models;

/// <summary>
/// Maps service-authoritative status into UI protection states.
/// Does not infer Active from desiredProtection alone.
/// </summary>
public static class ProtectionStateMapper
{
    public static ProtectionUiState Map(ServiceStatusSnapshot? status, bool serviceReachable)
    {
        if (!serviceReachable || status is null)
            return ProtectionUiState.ServiceUnavailable;

        var engine = status.Engine.State ?? "";
        var dns = status.Dns;

        if (dns.RecoveryRequired || string.Equals(engine, "RECOVERY_REQUIRED", StringComparison.OrdinalIgnoreCase))
            return ProtectionUiState.RecoveryRequired;

        if (string.Equals(engine, "DEGRADED", StringComparison.OrdinalIgnoreCase))
            return ProtectionUiState.Degraded;

        if (string.Equals(engine, "STARTING", StringComparison.OrdinalIgnoreCase))
            return ProtectionUiState.Starting;

        if (string.Equals(engine, "STOPPING", StringComparison.OrdinalIgnoreCase))
            return ProtectionUiState.Stopping;

        // Active requires runtime ACTIVE + filtering + healthy listener + ownership.
        if (string.Equals(engine, "ACTIVE", StringComparison.OrdinalIgnoreCase)
            && status.Engine.FilteringEnabled
            && status.Engine.ListenerHealthy
            && string.Equals(dns.DnsOwnership, "OWNED", StringComparison.OrdinalIgnoreCase))
        {
            return ProtectionUiState.Active;
        }

        // ACTIVE engine without ownership is inconsistent — surface as degraded/recovery.
        if (string.Equals(engine, "ACTIVE", StringComparison.OrdinalIgnoreCase)
            && status.Engine.FilteringEnabled
            && !string.Equals(dns.DnsOwnership, "OWNED", StringComparison.OrdinalIgnoreCase))
        {
            return dns.RecoveryRequired ? ProtectionUiState.RecoveryRequired : ProtectionUiState.Degraded;
        }

        if (string.Equals(engine, "DISABLED", StringComparison.OrdinalIgnoreCase)
            || !status.Engine.FilteringEnabled)
        {
            return ProtectionUiState.Off;
        }

        return ProtectionUiState.Off;
    }

    public static string ToDisplay(ProtectionUiState state) => state switch
    {
        ProtectionUiState.Off => "Off",
        ProtectionUiState.Starting => "Starting",
        ProtectionUiState.Active => "Active",
        ProtectionUiState.Degraded => "Degraded",
        ProtectionUiState.RecoveryRequired => "Recovery required",
        ProtectionUiState.Stopping => "Stopping",
        ProtectionUiState.ServiceUnavailable => "Service unavailable",
        _ => state.ToString()
    };

    public static string Guidance(ProtectionUiState state, ServiceStatusSnapshot? status) => state switch
    {
        ProtectionUiState.ServiceUnavailable =>
            "BlockAdsService is not available.",
        ProtectionUiState.RecoveryRequired =>
            "BlockAds detected a DNS ownership state that could not be safely restored.",
        ProtectionUiState.Degraded =>
            status?.Engine.LastError is { Length: > 0 } err
                ? err
                : "DNS protection has been released. Automatic re-enable of DNS apply is disabled until you take action.",
        ProtectionUiState.Active => "Protection is active.",
        ProtectionUiState.Off => "Protection is off.",
        ProtectionUiState.Starting => "Starting protection…",
        ProtectionUiState.Stopping => "Stopping protection…",
        _ => ""
    };
}
