// SPDX-License-Identifier: GPL-3.0-or-later
//
// Derived from G-Helper's app/Mode/Modes.cs and app/Battery/BatteryControl.cs
// (https://github.com/seerge/g-helper, GPL-3.0-or-later): the platform
// profile and the charge limit are kept by the app, not read back from the
// firmware. Trimmed to the three profiles Zephyr Center offers.

using System.Text.Json;

namespace ZephyrBridge;

/// <summary>
/// PerformanceProfiles applies the firmware's platform profile and charge
/// limit, and remembers what it applied.
///
/// Neither can be asked for afterwards. DSTS on the mode endpoint answers with
/// the mask of profiles the machine offers, and the charge endpoint with a
/// packed word, so both read the same before and after a write. G-Helper keeps
/// the choices in its own configuration for the same reason, and so does the
/// bridge, so the window still shows them after a restart.
/// </summary>
sealed class PerformanceProfiles
{
    static readonly (string Name, int Code)[] Known =
    [
        ("balanced", AsusACPI.PerformanceBalanced),
        ("performance", AsusACPI.PerformanceTurbo),
        ("quiet", AsusACPI.PerformanceSilent),
    ];

    readonly AsusACPI acpi;
    readonly string statePath;
    int current;
    int chargeLimit;
    string pendingGpu;

    public PerformanceProfiles(AsusACPI acpi, string statePath)
    {
        this.acpi = acpi;
        this.statePath = statePath;
        var saved = Read();
        current = saved?.Profile ?? AsusACPI.PerformanceBalanced;
        chargeLimit = saved?.ChargeLimit ?? 100;
        pendingGpu = saved?.PendingGpu ?? "";
    }

    /// <summary>Current is the profile in use, as a firmware code.</summary>
    public int Current => current;

    /// <summary>ChargeLimit is the charge ceiling last applied.</summary>
    public int ChargeLimit => chargeLimit;

    /// <summary>RememberChargeLimit records a charge ceiling the firmware
    /// accepted, so the next snapshot reports it.</summary>
    public void RememberChargeLimit(int limit)
    {
        chargeLimit = limit;
        Save();
    }

    /// <summary>PendingGpu is the graphics mode waiting for a restart, or ""
    /// when nothing is queued. The MUX only moves at boot, so the queue has
    /// to outlive the bridge process.</summary>
    public string PendingGpu
    {
        get => pendingGpu;
        set
        {
            if (pendingGpu == value) return;
            pendingGpu = value;
            Save();
        }
    }

    /// <summary>Name is the profile the app knows a firmware code by.</summary>
    public static string Name(int code) => code switch
    {
        AsusACPI.PerformanceTurbo => "performance",
        AsusACPI.PerformanceSilent => "quiet",
        _ => "balanced",
    };

    /// <summary>Code is the firmware code of a profile the app names.</summary>
    public static int Code(string profile) => profile switch
    {
        "performance" => AsusACPI.PerformanceTurbo,
        "quiet" => AsusACPI.PerformanceSilent,
        _ => AsusACPI.PerformanceBalanced,
    };

    /// <summary>Choices lists the profiles the machine offers, in the order
    /// the app shows them. The mode endpoint answers with a mask of options
    /// rather than a setting, and machines disagree on what it means, so a
    /// profile is offered when the firmware implements the endpoint.</summary>
    public List<string> Choices()
    {
        if (!acpi.DevicePresent(AsusACPI.PerformanceMode)) return ["balanced"];
        return ["quiet", "balanced", "performance"];
    }

    /// <summary>Apply writes a profile and remembers it once the firmware
    /// accepts it. The write is repeated once, because the EC drops a mode
    /// change it is still busy with.</summary>
    public bool Apply(int code)
    {
        for (int attempt = 0; attempt < 2; attempt++)
        {
            if (acpi.DeviceSet(AsusACPI.PerformanceMode, code) == 1)
            {
                current = code;
                Save();
                return true;
            }
            Thread.Sleep(120);
        }
        return false;
    }

    /// <summary>Restore re-applies the remembered profile at startup, so the
    /// machine is in the state the window shows.</summary>
    public void Restore()
    {
        acpi.DeviceSet(AsusACPI.PerformanceMode, current);
    }

    State? Read()
    {
        try
        {
            var saved = JsonSerializer.Deserialize<State>(File.ReadAllText(statePath));
            if (saved is null) return null;
            if (Array.FindIndex(Known, entry => entry.Code == saved.Profile) < 0)
                saved.Profile = AsusACPI.PerformanceBalanced;
            if (saved.ChargeLimit is < 20 or > 100) saved.ChargeLimit = 100;
            saved.PendingGpu = saved.PendingGpu switch
            {
                "integrated" => "integrated",
                "hybrid" => "hybrid",
                "ultimate" => "ultimate",
                _ => "",
            };
            return saved;
        }
        catch
        {
            // No saved profile yet, or the file cannot be read.
            return null;
        }
    }

    void Save()
    {
        try
        {
            string? dir = Path.GetDirectoryName(statePath);
            if (!string.IsNullOrEmpty(dir)) Directory.CreateDirectory(dir);
            File.WriteAllText(statePath, JsonSerializer.Serialize(
                new State { Profile = current, ChargeLimit = chargeLimit, PendingGpu = pendingGpu }));
        }
        catch
        {
            // A read-only config directory must not fail a hardware write.
        }
    }

    sealed class State
    {
        public int Profile { get; set; }
        public int ChargeLimit { get; set; } = 100;
        public string PendingGpu { get; set; } = "";
    }
}
