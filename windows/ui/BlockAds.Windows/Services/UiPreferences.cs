using System.IO;
using System.Text.Json;

namespace BlockAds.Windows.Services;

/// <summary>UI-only preferences under LocalAppData. Never stores protection state.</summary>
public sealed class UiPreferences
{
    public double? Left { get; set; }
    public double? Top { get; set; }
    public double? Width { get; set; }
    public double? Height { get; set; }
    public bool MinimizeToTray { get; set; } = true;

    private static string Path =>
        System.IO.Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "BlockAds", "ui-preferences.json");

    public static UiPreferences Load()
    {
        try
        {
            var p = Path;
            if (!File.Exists(p)) return new UiPreferences();
            return JsonSerializer.Deserialize<UiPreferences>(File.ReadAllText(p)) ?? new UiPreferences();
        }
        catch { return new UiPreferences(); }
    }

    public void Save()
    {
        try
        {
            var dir = System.IO.Path.GetDirectoryName(Path)!;
            Directory.CreateDirectory(dir);
            File.WriteAllText(Path, JsonSerializer.Serialize(this, new JsonSerializerOptions { WriteIndented = true }));
        }
        catch { /* ignore */ }
    }
}
