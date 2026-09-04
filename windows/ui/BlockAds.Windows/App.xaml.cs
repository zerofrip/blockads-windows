using System.IO;
using System.Threading;
using System.Windows;
using BlockAds.Windows.Services;
using BlockAds.Windows.ViewModels;

namespace BlockAds.Windows;

public partial class App : System.Windows.Application
{
    public const string UiMutexName = @"Local\BlockAds.Windows.UI";

    private Mutex? _mutex;
    private MainViewModel? _vm;
    private TrayService? _tray;
    private MainWindow? _main;
    private bool _exitStarted;

    protected override async void OnStartup(StartupEventArgs e)
    {
        base.OnStartup(e);

        _mutex = new Mutex(true, UiMutexName, out var created);
        if (!created)
        {
            // Second instance: exit (activation of existing instance is best-effort only).
            System.Windows.MessageBox.Show(
                "BlockAds is already running in the notification area.",
                "BlockAds",
                MessageBoxButton.OK,
                MessageBoxImage.Information);
            Shutdown(0);
            return;
        }

        // Automated VM smoke: exercises the same ViewModel → IPC path without tray UI.
        // Example: BlockAds.Windows.exe --smoke status|enable|disable|exit-check
        if (e.Args.Length >= 2 && e.Args[0] == "--smoke")
        {
            await RunSmokeAsync(e.Args[1]);
            _mutex.ReleaseMutex();
            _mutex.Dispose();
            Shutdown(Environment.ExitCode);
            return;
        }

        var client = new BlockAdsIpcClient();
        _vm = new MainViewModel(client);
        _main = new MainWindow(_vm);
        _tray = new TrayService(_vm, ShowMain, ExitUiAsync);
        _tray.Start();
        _vm.ActivateRequested += ShowMain;

        ShowMain();
        await _vm.StartAsync();
    }

    private static async Task RunSmokeAsync(string action)
    {
        var logPath = Path.Combine(@"C:\BlockAds-val\logs", "ui_smoke.txt");
        try { Directory.CreateDirectory(Path.GetDirectoryName(logPath)!); } catch { /* ignore */ }
        void W(string m)
        {
            try { File.AppendAllText(logPath, m + Environment.NewLine); } catch { /* ignore */ }
            try { Console.WriteLine(m); } catch { /* ignore */ }
        }

        var client = new BlockAdsIpcClient();
        var vm = new MainViewModel(client, TimeSpan.FromHours(1));
        try
        {
            switch (action.ToLowerInvariant())
            {
                case "status":
                    await vm.RefreshAsync();
                    W("SMOKE_PROTECTION=" + vm.ProtectionDisplay);
                    W("SMOKE_SERVICE=" + vm.ServiceState);
                    W("SMOKE_OWNERSHIP=" + vm.DnsOwnership);
                    W("SMOKE_RECOVERY=" + vm.RecoveryState);
                    Environment.ExitCode = vm.ProtectionState == Models.ProtectionUiState.ServiceUnavailable ? 2 : 0;
                    break;
                case "enable":
                    await vm.RefreshAsync();
                    if (vm.EnableCommand.CanExecute(null)) vm.EnableCommand.Execute(null);
                    await Task.Delay(8000);
                    await vm.RefreshAsync();
                    W("SMOKE_PROTECTION=" + vm.ProtectionDisplay);
                    W("SMOKE_ENABLE_INVOKED=" + vm.EnableInvoked);
                    Environment.ExitCode = vm.ProtectionState == Models.ProtectionUiState.Active ? 0 : 3;
                    break;
                case "disable":
                    await vm.RefreshAsync();
                    if (vm.DisableCommand.CanExecute(null)) vm.DisableCommand.Execute(null);
                    await Task.Delay(8000);
                    await vm.RefreshAsync();
                    W("SMOKE_PROTECTION=" + vm.ProtectionDisplay);
                    W("SMOKE_DISABLE_INVOKED=" + vm.DisableInvoked);
                    Environment.ExitCode = vm.ProtectionState == Models.ProtectionUiState.Off ? 0 : 4;
                    break;
                case "exit-check":
                    await vm.RefreshAsync();
                    await vm.ExitUiAsync();
                    W("SMOKE_EXIT_DISABLE_INVOKED=" + vm.DisableInvoked);
                    Environment.ExitCode = !vm.DisableInvoked ? 0 : 5;
                    break;
                case "unavailable":
                    await vm.RefreshAsync();
                    W("SMOKE_PROTECTION=" + vm.ProtectionDisplay);
                    W("SMOKE_GUIDANCE=" + vm.Guidance);
                    Environment.ExitCode = vm.ProtectionState == Models.ProtectionUiState.ServiceUnavailable ? 0 : 6;
                    break;
                case "recovery":
                    // Renders whatever the service reports; for injection use status DTO path in unit tests.
                    await vm.RefreshAsync();
                    W("SMOKE_PROTECTION=" + vm.ProtectionDisplay);
                    W("SMOKE_GUIDANCE=" + vm.Guidance);
                    Environment.ExitCode = 0;
                    break;
                default:
                    W("SMOKE_UNKNOWN_ACTION=" + action);
                    Environment.ExitCode = 1;
                    break;
            }
            W("SMOKE_EXITCODE=" + Environment.ExitCode);
        }
        finally
        {
            await vm.DisposeAsync();
        }
    }

    private void ShowMain()
    {
        if (_main is null) return;
        _main.Show();
        _main.WindowState = WindowState.Normal;
        _main.Activate();
    }

    private async Task ExitUiAsync()
    {
        if (_exitStarted) return;
        _exitStarted = true;
        // Exit UI only — do not disable filtering or stop the service.
        if (_vm is not null)
            await _vm.ExitUiAsync();
        _tray?.Dispose();
        _mutex?.ReleaseMutex();
        _mutex?.Dispose();
        Shutdown(0);
    }

    protected override void OnExit(ExitEventArgs e)
    {
        _tray?.Dispose();
        _mutex?.Dispose();
        base.OnExit(e);
    }
}
