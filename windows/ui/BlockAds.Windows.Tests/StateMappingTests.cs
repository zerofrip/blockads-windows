using BlockAds.Windows.Models;
using BlockAds.Windows.Services;
using BlockAds.Windows.ViewModels;
using Xunit;

namespace BlockAds.Windows.Tests;

public class ProtectionStateMapperTests
{
    [Fact]
    public void ServiceUnavailable_WhenUnreachable()
    {
        Assert.Equal(ProtectionUiState.ServiceUnavailable, ProtectionStateMapper.Map(null, false));
    }

    [Fact]
    public void Active_RequiresOwnershipAndHealthyListener()
    {
        var s = Snapshot("ACTIVE", filtering: true, healthy: true, ownership: "OWNED");
        Assert.Equal(ProtectionUiState.Active, ProtectionStateMapper.Map(s, true));
    }

    [Fact]
    public void DesiredEnabledAlone_IsNotActive()
    {
        var s = Snapshot("DISABLED", filtering: false, healthy: false, ownership: "NONE");
        s.Dns.DesiredProtection = "ENABLED";
        Assert.Equal(ProtectionUiState.Off, ProtectionStateMapper.Map(s, true));
    }

    [Fact]
    public void ActiveWithoutOwnership_IsDegraded()
    {
        var s = Snapshot("ACTIVE", filtering: true, healthy: true, ownership: "NONE");
        Assert.Equal(ProtectionUiState.Degraded, ProtectionStateMapper.Map(s, true));
    }

    [Fact]
    public void RecoveryRequired_IsDistinct()
    {
        var s = Snapshot("RECOVERY_REQUIRED", filtering: false, healthy: false, ownership: "NONE");
        s.Dns.RecoveryRequired = true;
        Assert.Equal(ProtectionUiState.RecoveryRequired, ProtectionStateMapper.Map(s, true));
    }

    [Fact]
    public void Degraded_Maps()
    {
        var s = Snapshot("DEGRADED", filtering: false, healthy: false, ownership: "NONE");
        Assert.Equal(ProtectionUiState.Degraded, ProtectionStateMapper.Map(s, true));
    }

    [Fact]
    public void ListenerUnhealthyActive_NotMappedAsActive()
    {
        var s = Snapshot("ACTIVE", filtering: true, healthy: false, ownership: "OWNED");
        Assert.NotEqual(ProtectionUiState.Active, ProtectionStateMapper.Map(s, true));
    }

    private static ServiceStatusSnapshot Snapshot(string engine, bool filtering, bool healthy, string ownership) =>
        new()
        {
            Engine = new EngineStatusDto
            {
                State = engine,
                FilteringEnabled = filtering,
                ListenerHealthy = healthy
            },
            Dns = new DnsStatusDto
            {
                DesiredProtection = filtering ? "ENABLED" : "DISABLED",
                RuntimeProtection = engine,
                DnsOwnership = ownership,
                State = engine
            }
        };
}

public sealed class FakeIpcClient : IBlockAdsIpcClient
{
    public ServiceStatusSnapshot StatusValue { get; set; } = new();
    public bool ThrowUnavailable { get; set; }
    public int StatusCalls { get; private set; }
    public int EnableCalls { get; private set; }
    public int DisableCalls { get; private set; }
    public int ReloadCalls { get; private set; }

    public Task<bool> PingAsync(CancellationToken ct = default) =>
        ThrowUnavailable
            ? Task.FromException<bool>(new BlockAdsIpcException(UiErrorCategory.ServiceUnavailable, "down"))
            : Task.FromResult(true);

    public Task<ServiceStatusSnapshot> StatusAsync(CancellationToken ct = default)
    {
        StatusCalls++;
        if (ThrowUnavailable)
            throw new BlockAdsIpcException(UiErrorCategory.ServiceUnavailable, "BlockAdsService is not available.");
        return Task.FromResult(StatusValue);
    }

    public Task<ServiceStatusSnapshot> EnableAsync(CancellationToken ct = default)
    {
        EnableCalls++;
        StatusValue.Engine.State = "ACTIVE";
        StatusValue.Engine.FilteringEnabled = true;
        StatusValue.Engine.ListenerHealthy = true;
        StatusValue.Dns.DesiredProtection = "ENABLED";
        StatusValue.Dns.RuntimeProtection = "ACTIVE";
        StatusValue.Dns.DnsOwnership = "OWNED";
        return Task.FromResult(StatusValue);
    }

    public Task<ServiceStatusSnapshot> DisableAsync(CancellationToken ct = default)
    {
        DisableCalls++;
        StatusValue.Engine.State = "DISABLED";
        StatusValue.Engine.FilteringEnabled = false;
        StatusValue.Engine.ListenerHealthy = false;
        StatusValue.Dns.DesiredProtection = "DISABLED";
        StatusValue.Dns.RuntimeProtection = "DISABLED";
        StatusValue.Dns.DnsOwnership = "NONE";
        return Task.FromResult(StatusValue);
    }

    public Task ReloadFiltersAsync(CancellationToken ct = default)
    {
        ReloadCalls++;
        return Task.CompletedTask;
    }

    public Task<System.Text.Json.JsonElement> GetStatsAsync(CancellationToken ct = default) =>
        Task.FromResult(System.Text.Json.JsonDocument.Parse("{}").RootElement.Clone());

    public ValueTask DisposeAsync() => ValueTask.CompletedTask;
}

public class MainViewModelLifecycleTests
{
    [Fact]
    public async Task ExitUi_DoesNotInvokeDisable()
    {
        var fake = new FakeIpcClient
        {
            StatusValue = new ServiceStatusSnapshot
            {
                Engine = new EngineStatusDto { State = "ACTIVE", FilteringEnabled = true, ListenerHealthy = true },
                Dns = new DnsStatusDto { DesiredProtection = "ENABLED", RuntimeProtection = "ACTIVE", DnsOwnership = "OWNED" }
            }
        };
        var vm = new MainViewModel(fake, TimeSpan.FromHours(1));
        await vm.RefreshAsync();
        Assert.Equal(ProtectionUiState.Active, vm.ProtectionState);
        await vm.ExitUiAsync();
        Assert.Equal(0, fake.DisableCalls);
        Assert.False(vm.DisableInvoked);
    }

    [Fact]
    public async Task Polling_OnlyCallsStatus()
    {
        var fake = new FakeIpcClient
        {
            StatusValue = new ServiceStatusSnapshot
            {
                Engine = new EngineStatusDto { State = "DISABLED" },
                Dns = new DnsStatusDto { DesiredProtection = "DISABLED", RuntimeProtection = "DISABLED", DnsOwnership = "NONE" }
            }
        };
        var vm = new MainViewModel(fake, TimeSpan.FromMilliseconds(50));
        await vm.StartAsync();
        await Task.Delay(200);
        await vm.StopPollingAsync();
        Assert.True(fake.StatusCalls >= 2);
        Assert.Equal(0, fake.EnableCalls);
        Assert.Equal(0, fake.DisableCalls);
        Assert.Equal(0, fake.ReloadCalls);
    }

    [Fact]
    public async Task ServiceUnavailable_RendersDistinctState()
    {
        var fake = new FakeIpcClient { ThrowUnavailable = true };
        var vm = new MainViewModel(fake, TimeSpan.FromHours(1));
        await vm.RefreshAsync();
        Assert.Equal(ProtectionUiState.ServiceUnavailable, vm.ProtectionState);
        Assert.Contains("not available", vm.Guidance, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task EnableTransition_UsesAuthoritativeStatus()
    {
        var fake = new FakeIpcClient
        {
            StatusValue = new ServiceStatusSnapshot
            {
                Engine = new EngineStatusDto { State = "DISABLED" },
                Dns = new DnsStatusDto { DesiredProtection = "DISABLED", RuntimeProtection = "DISABLED", DnsOwnership = "NONE" }
            }
        };
        var vm = new MainViewModel(fake, TimeSpan.FromHours(1));
        await vm.RefreshAsync();
        Assert.True(vm.EnableCommand.CanExecute(null));
        vm.EnableCommand.Execute(null);
        await Task.Delay(100);
        Assert.True(vm.EnableInvoked);
        Assert.Equal(1, fake.EnableCalls);
        Assert.Equal(ProtectionUiState.Active, vm.ProtectionState);
    }

    [Fact]
    public async Task RecoveryRequired_RendersDistinctly()
    {
        var fake = new FakeIpcClient
        {
            StatusValue = new ServiceStatusSnapshot
            {
                Engine = new EngineStatusDto { State = "RECOVERY_REQUIRED" },
                Dns = new DnsStatusDto
                {
                    DesiredProtection = "ENABLED",
                    RuntimeProtection = "RECOVERY_REQUIRED",
                    DnsOwnership = "NONE",
                    RecoveryRequired = true
                }
            }
        };
        var vm = new MainViewModel(fake, TimeSpan.FromHours(1));
        await vm.RefreshAsync();
        Assert.Equal(ProtectionUiState.RecoveryRequired, vm.ProtectionState);
        Assert.Contains("could not be safely restored", vm.Guidance, StringComparison.OrdinalIgnoreCase);
    }
}
