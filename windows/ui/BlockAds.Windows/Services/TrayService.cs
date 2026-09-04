using System.Drawing;
using System.Windows.Forms;
using BlockAds.Windows.ViewModels;
using WpfApp = System.Windows.Application;

namespace BlockAds.Windows.Services;

/// <summary>
/// Notification-area icon. Exit UI does not disable protection or stop the service.
/// </summary>
public sealed class TrayService : IDisposable
{
    private readonly MainViewModel _vm;
    private readonly Action _showMain;
    private readonly Func<Task> _exitUi;
    private NotifyIcon? _icon;

    public TrayService(MainViewModel vm, Action showMain, Func<Task> exitUi)
    {
        _vm = vm;
        _showMain = showMain;
        _exitUi = exitUi;
    }

    public void Start()
    {
        _icon = new NotifyIcon
        {
            Text = "BlockAds",
            Visible = true,
            Icon = SystemIcons.Shield
        };
        _icon.DoubleClick += (_, _) => _showMain();
        RebuildMenu();
        _vm.PropertyChanged += (_, e) =>
        {
            if (e.PropertyName is nameof(MainViewModel.ProtectionDisplay) or nameof(MainViewModel.ProtectionState))
                WpfApp.Current?.Dispatcher.Invoke(RebuildMenu);
        };
    }

    private void RebuildMenu()
    {
        if (_icon is null) return;
        var menu = new ContextMenuStrip();
        menu.Items.Add("Open BlockAds", null, (_, _) => _showMain());
        menu.Items.Add($"Protection: {_vm.ProtectionDisplay}").Enabled = false;
        menu.Items.Add(new ToolStripSeparator());
        var enableItem = new ToolStripMenuItem("Enable");
        enableItem.Enabled = _vm.EnableCommand.CanExecute(null);
        enableItem.Click += (_, _) => { if (_vm.EnableCommand.CanExecute(null)) _vm.EnableCommand.Execute(null); };
        menu.Items.Add(enableItem);
        var disableItem = new ToolStripMenuItem("Disable");
        disableItem.Enabled = _vm.DisableCommand.CanExecute(null);
        disableItem.Click += (_, _) => { if (_vm.DisableCommand.CanExecute(null)) _vm.DisableCommand.Execute(null); };
        menu.Items.Add(disableItem);
        menu.Items.Add("Refresh", null, (_, _) =>
        {
            if (_vm.RefreshCommand.CanExecute(null)) _vm.RefreshCommand.Execute(null);
        });
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Exit UI", null, (_, _) => { _ = _exitUi(); });
        _icon.ContextMenuStrip = menu;
        _icon.Text = $"BlockAds — {_vm.ProtectionDisplay}";
    }

    public void Dispose()
    {
        if (_icon is null) return;
        _icon.Visible = false;
        _icon.Dispose();
        _icon = null;
    }
}
