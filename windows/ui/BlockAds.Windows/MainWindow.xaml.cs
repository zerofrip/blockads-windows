using System.ComponentModel;
using System.Windows;
using BlockAds.Windows.Services;
using BlockAds.Windows.ViewModels;

namespace BlockAds.Windows;

public partial class MainWindow : Window
{
    private readonly MainViewModel _vm;
    private readonly UiPreferences _prefs;
    private bool _forceClose;

    public MainWindow(MainViewModel vm)
    {
        InitializeComponent();
        _vm = vm;
        DataContext = vm;
        _prefs = UiPreferences.Load();
        if (_prefs.Left is not null) Left = _prefs.Left.Value;
        if (_prefs.Top is not null) Top = _prefs.Top.Value;
        if (_prefs.Width is not null) Width = _prefs.Width.Value;
        if (_prefs.Height is not null) Height = _prefs.Height.Value;
    }

    public void ForceClose()
    {
        _forceClose = true;
        Close();
    }

    protected override void OnClosing(CancelEventArgs e)
    {
        _prefs.Left = Left;
        _prefs.Top = Top;
        _prefs.Width = Width;
        _prefs.Height = Height;
        _prefs.Save();

        if (!_forceClose && _prefs.MinimizeToTray)
        {
            e.Cancel = true;
            Hide();
            return;
        }
        base.OnClosing(e);
    }
}
