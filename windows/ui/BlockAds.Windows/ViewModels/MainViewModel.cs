using System.ComponentModel;
using System.Runtime.CompilerServices;
using System.Windows.Input;
using BlockAds.Windows.Models;
using BlockAds.Windows.Services;

namespace BlockAds.Windows.ViewModels;

public interface IUiMutationGuard
{
    bool DisableInvoked { get; }
    bool EnableInvoked { get; }
}

public sealed class RelayCommand : ICommand
{
    private readonly Func<object?, bool>? _can;
    private readonly Func<object?, Task> _exec;
    private bool _running;

    public RelayCommand(Func<object?, Task> exec, Func<object?, bool>? can = null)
    {
        _exec = exec;
        _can = can;
    }

    public bool CanExecute(object? parameter) => !_running && (_can?.Invoke(parameter) ?? true);
    public event EventHandler? CanExecuteChanged;
    public void RaiseCanExecuteChanged() => CanExecuteChanged?.Invoke(this, EventArgs.Empty);

    public async void Execute(object? parameter)
    {
        if (!CanExecute(parameter)) return;
        _running = true;
        RaiseCanExecuteChanged();
        try { await _exec(parameter); }
        finally
        {
            _running = false;
            RaiseCanExecuteChanged();
        }
    }
}

public sealed class MainViewModel : INotifyPropertyChanged, IUiMutationGuard, IAsyncDisposable
{
    private readonly IBlockAdsIpcClient _client;
    private readonly TimeSpan _pollInterval;
    private CancellationTokenSource? _pollCts;
    private Task? _pollTask;
    private int _pollInFlight;
    private bool _busy;
    private ProtectionUiState _protection = ProtectionUiState.ServiceUnavailable;
    private string _guidance = ProtectionStateMapper.Guidance(ProtectionUiState.ServiceUnavailable, null);
    private string _serviceState = "—";
    private string _desired = "—";
    private string _runtime = "—";
    private string _listener = "—";
    private string _ownership = "—";
    private string _recovery = "—";
    private string _filters = "—";
    private string _lastError = "";
    private string _lastHealthFailure = "";
    private string _lastRestoreError = "";
    private string _lastNetworkChange = "";
    private string _statusDetail = "";
    private ServiceStatusSnapshot? _lastStatus;
    private bool _serviceReachable;

    public MainViewModel(IBlockAdsIpcClient client, TimeSpan? pollInterval = null)
    {
        _client = client;
        _pollInterval = pollInterval ?? TimeSpan.FromSeconds(3);
        EnableCommand = new RelayCommand(_ => EnableAsync(), _ => CanEnable());
        DisableCommand = new RelayCommand(_ => DisableAsync(), _ => CanDisable());
        RefreshCommand = new RelayCommand(_ => RefreshAsync(), _ => !_busy);
        ReloadFiltersCommand = new RelayCommand(_ => ReloadFiltersAsync(), _ => !_busy && _serviceReachable && _protection == ProtectionUiState.Active);
    }

    public bool DisableInvoked { get; private set; }
    public bool EnableInvoked { get; private set; }

    public ICommand EnableCommand { get; }
    public ICommand DisableCommand { get; }
    public ICommand RefreshCommand { get; }
    public ICommand ReloadFiltersCommand { get; }

    public ProtectionUiState ProtectionState
    {
        get => _protection;
        private set { if (Set(ref _protection, value)) OnPropertyChanged(nameof(ProtectionDisplay)); }
    }

    public string ProtectionDisplay => ProtectionStateMapper.ToDisplay(ProtectionState);
    public string Guidance { get => _guidance; private set => Set(ref _guidance, value); }
    public string ServiceState { get => _serviceState; private set => Set(ref _serviceState, value); }
    public string DesiredProtection { get => _desired; private set => Set(ref _desired, value); }
    public string RuntimeProtection { get => _runtime; private set => Set(ref _runtime, value); }
    public string ListenerHealth { get => _listener; private set => Set(ref _listener, value); }
    public string DnsOwnership { get => _ownership; private set => Set(ref _ownership, value); }
    public string RecoveryState { get => _recovery; private set => Set(ref _recovery, value); }
    public string FilterStatus { get => _filters; private set => Set(ref _filters, value); }
    public string LastError { get => _lastError; private set => Set(ref _lastError, value); }
    public string LastHealthFailure { get => _lastHealthFailure; private set => Set(ref _lastHealthFailure, value); }
    public string LastRestoreError { get => _lastRestoreError; private set => Set(ref _lastRestoreError, value); }
    public string LastNetworkChange { get => _lastNetworkChange; private set => Set(ref _lastNetworkChange, value); }
    public string StatusDetail { get => _statusDetail; private set => Set(ref _statusDetail, value); }
    public bool IsBusy { get => _busy; private set { if (Set(ref _busy, value)) RaiseCommands(); } }
    public bool ServiceReachable { get => _serviceReachable; private set => Set(ref _serviceReachable, value); }

    public event PropertyChangedEventHandler? PropertyChanged;
    public event Action? ActivateRequested;

    public void RequestActivate() => ActivateRequested?.Invoke();

    public async Task StartAsync()
    {
        await RefreshAsync().ConfigureAwait(true);
        _pollCts = new CancellationTokenSource();
        _pollTask = PollLoopAsync(_pollCts.Token);
    }

    public async Task StopPollingAsync()
    {
        if (_pollCts is null) return;
        _pollCts.Cancel();
        try { if (_pollTask is not null) await _pollTask.ConfigureAwait(false); } catch { /* ignore */ }
        _pollCts.Dispose();
        _pollCts = null;
    }

    /// <summary>Exit UI must not call Disable.</summary>
    public async Task ExitUiAsync()
    {
        await StopPollingAsync().ConfigureAwait(false);
    }

    private bool CanEnable() =>
        !_busy && _serviceReachable &&
        ProtectionState is ProtectionUiState.Off or ProtectionUiState.Degraded;

    private bool CanDisable() =>
        !_busy && _serviceReachable &&
        ProtectionState is ProtectionUiState.Active or ProtectionUiState.Starting
            or ProtectionUiState.Degraded or ProtectionUiState.RecoveryRequired;

    private async Task EnableAsync()
    {
        EnableInvoked = true;
        IsBusy = true;
        ProtectionState = ProtectionUiState.Starting;
        Guidance = ProtectionStateMapper.Guidance(ProtectionUiState.Starting, _lastStatus);
        try
        {
            var status = await _client.EnableAsync().ConfigureAwait(true);
            ApplyStatus(status, reachable: true);
            LastError = "";
        }
        catch (BlockAdsIpcException ex)
        {
            LastError = $"{ex.Category}: {ex.Message}";
            await SafeRefreshAfterErrorAsync().ConfigureAwait(true);
        }
        finally { IsBusy = false; }
    }

    private async Task DisableAsync()
    {
        DisableInvoked = true;
        IsBusy = true;
        ProtectionState = ProtectionUiState.Stopping;
        Guidance = ProtectionStateMapper.Guidance(ProtectionUiState.Stopping, _lastStatus);
        try
        {
            var status = await _client.DisableAsync().ConfigureAwait(true);
            ApplyStatus(status, reachable: true);
            LastError = "";
        }
        catch (BlockAdsIpcException ex)
        {
            LastError = $"{ex.Category}: {ex.Message}";
            await SafeRefreshAfterErrorAsync().ConfigureAwait(true);
        }
        finally { IsBusy = false; }
    }

    private async Task ReloadFiltersAsync()
    {
        IsBusy = true;
        try
        {
            await _client.ReloadFiltersAsync().ConfigureAwait(true);
            await RefreshAsync().ConfigureAwait(true);
        }
        catch (BlockAdsIpcException ex)
        {
            LastError = $"{UiErrorCategory.FilterReloadFailed}: {ex.Message}";
            await SafeRefreshAfterErrorAsync().ConfigureAwait(true);
        }
        finally { IsBusy = false; }
    }

    public async Task RefreshAsync()
    {
        try
        {
            var status = await _client.StatusAsync().ConfigureAwait(true);
            ApplyStatus(status, reachable: true);
            if (ProtectionState != ProtectionUiState.ServiceUnavailable)
                LastError = string.IsNullOrEmpty(status.Engine.LastError) ? LastError : status.Engine.LastError!;
        }
        catch (BlockAdsIpcException ex) when (ex.Category is UiErrorCategory.ServiceUnavailable or UiErrorCategory.Timeout)
        {
            ApplyUnavailable("BlockAdsService is not available.");
        }
        catch (BlockAdsIpcException ex)
        {
            LastError = ex.Message;
            ApplyUnavailable(ex.Message);
        }
        finally { RaiseCommands(); }
    }

    private async Task SafeRefreshAfterErrorAsync()
    {
        try
        {
            var status = await _client.StatusAsync().ConfigureAwait(true);
            ApplyStatus(status, reachable: true);
        }
        catch
        {
            ApplyUnavailable(string.IsNullOrEmpty(LastError) ? "BlockAdsService is not available." : LastError);
        }
    }

    private async Task PollLoopAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try { await Task.Delay(_pollInterval, ct).ConfigureAwait(true); }
            catch (OperationCanceledException) { break; }
            if (_busy) continue;
            if (Interlocked.CompareExchange(ref _pollInFlight, 1, 0) != 0) continue;
            try
            {
                var status = await _client.StatusAsync(ct).ConfigureAwait(true);
                ApplyStatus(status, reachable: true);
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested) { break; }
            catch
            {
                ApplyUnavailable("BlockAdsService is not available.");
            }
            finally
            {
                Interlocked.Exchange(ref _pollInFlight, 0);
                RaiseCommands();
            }
        }
    }

    private void ApplyUnavailable(string message)
    {
        ServiceReachable = false;
        _lastStatus = null;
        ProtectionState = ProtectionUiState.ServiceUnavailable;
        Guidance = string.IsNullOrWhiteSpace(message)
            ? ProtectionStateMapper.Guidance(ProtectionUiState.ServiceUnavailable, null)
            : message;
        ServiceState = "Unavailable";
        DesiredProtection = "—";
        RuntimeProtection = "—";
        ListenerHealth = "—";
        DnsOwnership = "—";
        RecoveryState = "—";
        FilterStatus = "—";
    }

    private void ApplyStatus(ServiceStatusSnapshot status, bool reachable)
    {
        ServiceReachable = reachable;
        _lastStatus = status;
        ProtectionState = ProtectionStateMapper.Map(status, reachable);
        Guidance = ProtectionStateMapper.Guidance(ProtectionState, status);
        ServiceState = string.IsNullOrEmpty(status.Service.State) ? "running" : status.Service.State;
        DesiredProtection = status.Dns.DesiredProtection;
        RuntimeProtection = status.Dns.RuntimeProtection;
        ListenerHealth = status.Engine.ListenerHealthy ? "Healthy" : "Unhealthy";
        DnsOwnership = status.Dns.DnsOwnership;
        RecoveryState = status.Dns.RecoveryRequired ? "Required" : "None";
        if (status.Filters.Loaded)
            FilterStatus = status.Filters.ListIds is { Count: > 0 }
                ? $"Loaded ({string.Join(", ", status.Filters.ListIds)})"
                : "Loaded";
        else
            FilterStatus = "Not loaded";
        if (!string.IsNullOrEmpty(status.Filters.LastUpdateError))
            FilterStatus += $" — error: {status.Filters.LastUpdateError}";
        LastHealthFailure = status.Dns.LastHealthFailure ?? "";
        LastRestoreError = status.Dns.LastRestoreError ?? "";
        LastNetworkChange = status.Dns.LastNetworkChange is { } t && t.Year > 1
            ? t.ToLocalTime().ToString("g")
            : "";
        StatusDetail = status.Engine.LastError ?? "";
    }

    private void RaiseCommands()
    {
        (EnableCommand as RelayCommand)?.RaiseCanExecuteChanged();
        (DisableCommand as RelayCommand)?.RaiseCanExecuteChanged();
        (RefreshCommand as RelayCommand)?.RaiseCanExecuteChanged();
        (ReloadFiltersCommand as RelayCommand)?.RaiseCanExecuteChanged();
    }

    private bool Set<T>(ref T field, T value, [CallerMemberName] string? name = null)
    {
        if (EqualityComparer<T>.Default.Equals(field, value)) return false;
        field = value;
        OnPropertyChanged(name);
        return true;
    }

    private void OnPropertyChanged([CallerMemberName] string? name = null) =>
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));

    public async ValueTask DisposeAsync()
    {
        await StopPollingAsync().ConfigureAwait(false);
        await _client.DisposeAsync().ConfigureAwait(false);
    }
}
