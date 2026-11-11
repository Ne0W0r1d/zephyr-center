// SPDX-License-Identifier: GPL-3.0-or-later
//
// Derived from G-Helper's app/USB/Aura.cs and app/USB/AsusHid.cs
// (https://github.com/seerge/g-helper, GPL-3.0-or-later), trimmed to the
// Aura controls Zephyr Center shows: effect, brightness, colour and the
// keyboard power states. ROG machines use the ASUS HID device; TUF and
// Vivobook/Zenbook/ProArt machines use the ACPI keyboard calls.

using System.Text.Json;
using HidSharp;
using HidSharp.Reports;

namespace ZephyrBridge;

/// <summary>AuraController drives the keyboard lighting on Windows.</summary>
public sealed class AuraController
{
    const int AsusVendorId = 0x0B05;
    const byte AuraId = 0x5D;
    const byte InputId = 0x5A;

    static readonly int[] AuraPids =
    [
        0x1A30, 0x1854, 0x1869, 0x1866, 0x19B6, 0x1822, 0x1837,
        0x184A, 0x183D, 0x8502, 0x1807, 0x17E0, 0x1ABE, 0x1B4C,
        0x1B6E, 0x1B2C, 0x8854, 0x1CE7, 0x1CE6, 0x1BF2, 0x1BF3,
        0x1CD7, 0x1CD8, 0x4B42,
    ];

    static readonly uint[] UsbModes = [0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12];
    static readonly uint[] AcpiModes = [0, 1, 2, 3, 10];
    static readonly uint[] Zones = [0];
    static readonly uint[] PowerZones = [1];
    static readonly uint[] Brightnesses = [0, 1, 2, 3];

    readonly AsusACPI acpi;
    readonly bool acpiAura;
    readonly string statePath;

    bool detected;
    bool usbAura;
    DateTime nextDetect;

    SavedState state = new();

    public AuraController(AsusACPI acpi, string model)
    {
        this.acpi = acpi;
        acpiAura = LooksLikeAcpiAura(model);
        statePath = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "ZephyrCenter", "aura.json");
        LoadState();
    }

    static bool LooksLikeAcpiAura(string model)
    {
        return model.Contains("TUF", StringComparison.OrdinalIgnoreCase)
            || model.Contains("Vivobook", StringComparison.OrdinalIgnoreCase)
            || model.Contains("Zenbook", StringComparison.OrdinalIgnoreCase)
            || model.Contains("ProArt", StringComparison.OrdinalIgnoreCase);
    }

    void Detect()
    {
        if (acpiAura) return;
        if (detected && (usbAura || DateTime.UtcNow < nextDetect)) return;
        detected = true;
        nextDetect = DateTime.UtcNow.AddSeconds(10);
        usbAura = FindDevices(AuraId).Count > 0;
    }

    public bool Supported
    {
        get
        {
            Detect();
            return acpiAura || usbAura;
        }
    }

    public AuraSnapshot Snapshot()
    {
        Detect();
        var modes = acpiAura ? AcpiModes : UsbModes;
        return new AuraSnapshot
        {
            Supported = Supported,
            Modes = [.. modes],
            Brightness = state.Brightness,
            Brightnesses = [.. Brightnesses],
            Mode = new AuraModeSnapshot
            {
                Mode = state.Mode,
                Zone = state.Zone,
                Color1 = [state.Red1, state.Green1, state.Blue1],
                Color2 = [state.Red2, state.Green2, state.Blue2],
                Speed = state.Speed,
                Direction = state.Direction,
            },
            Power =
            [
                new AuraPowerSnapshot
                {
                    Zone = 1,
                    Boot = state.Boot,
                    Awake = state.Awake,
                    Sleep = state.Sleep,
                    Shutdown = state.Shutdown,
                },
            ],
            PowerZones = [.. PowerZones],
            Zones = [.. Zones],
            DeviceType = 0,
            Path = acpiAura ? "acpi" : usbAura ? "hid" : "",
        };
    }

    public bool SetAura(uint mode, uint zone, int[] color1, int[] color2, string speed, string direction)
    {
        Detect();
        state.Mode = mode;
        state.Zone = zone;
        Rgb(color1, out byte red1, out byte green1, out byte blue1);
        Rgb(color2, out byte red2, out byte green2, out byte blue2);
        state.Red1 = red1;
        state.Green1 = green1;
        state.Blue1 = blue1;
        state.Red2 = red2;
        state.Green2 = green2;
        state.Blue2 = blue2;
        state.Speed = NormalizeSpeed(speed);
        state.Direction = NormalizeDirection(direction);
        SaveState();

        if (acpiAura)
        {
            return ApplyTufMode();
        }
        return ApplyUsbMode();
    }

    public bool SetBrightness(uint brightness)
    {
        Detect();
        state.Brightness = Math.Min(brightness, 3);
        SaveState();

        if (acpiAura)
        {
            return acpi.DeviceSet(AsusACPI.TUF_KB_BRIGHTNESS,
                (int)(0x80 | (state.Brightness & 0x7F))) == 1;
        }
        return WriteInput(
        [
            InputId, 0xBA, 0xC5, 0xC4, (byte)state.Brightness,
        ]);
    }

    public bool SetPower(uint zone, bool boot, bool awake, bool sleep, bool shutdown)
    {
        Detect();
        if (zone == 0 || zone == 1)
        {
            state.Boot = boot;
            state.Awake = awake;
            state.Sleep = sleep;
            state.Shutdown = shutdown;
        }
        SaveState();

        if (acpiAura)
        {
            return acpi.DeviceSet(AsusACPI.TUF_KB_STATE, TufPowerState(
                state.Boot, state.Awake, state.Sleep, state.Shutdown)) == 1;
        }

        byte keyb = 0;
        if (state.Boot) keyb |= 1 << 1;
        if (state.Awake) keyb |= 1 << 3;
        if (state.Sleep) keyb |= 1 << 5;
        if (state.Shutdown) keyb |= 1 << 7;
        return WriteAura([AuraId, 0xBD, 0x01, keyb, 0x00, 0x00, 0x00, 0xFF]);
    }

    bool ApplyUsbMode()
    {
        byte speed = SpeedByte(state.Speed);
        byte direction = DirectionByte(state.Direction);
        byte random = (byte)((state.Red1 == 0 && state.Green1 == 0 && state.Blue1 == 0)
            ? 0xFF
            : state.Mode == 1 ? 0x01 : 0x00);
        byte[] mode =
        [
            AuraId, 0xB3, (byte)state.Zone, (byte)state.Mode,
            state.Red1, state.Green1, state.Blue1, speed, direction, random,
            state.Red2, state.Green2, state.Blue2,
        ];
        return WriteAura(
            mode,
            [AuraId, 0xB5, 0x00, 0x00, 0x00],
            [AuraId, 0xB4]);
    }

    bool ApplyTufMode()
    {
        byte speed = SpeedByte(state.Speed);
        byte[] setting =
        [
            0xB4, (byte)state.Mode, state.Red1, state.Green1, state.Blue1, speed,
        ];
        int result = acpi.DeviceSet(AsusACPI.TUF_KB, setting);
        if (result == 1) return true;

        setting[0] = 0xB3;
        acpi.DeviceSet(AsusACPI.TUF_KB2, setting);
        setting[0] = 0xB4;
        return acpi.DeviceSet(AsusACPI.TUF_KB2, setting) == 1;
    }

    static int TufPowerState(bool boot, bool awake, bool sleep, bool shutdown)
    {
        int power = 0xBD;
        if (boot) power |= 0x03 << 16;
        if (awake) power |= 0x0C << 16;
        if (sleep) power |= 0x30 << 16;
        if (shutdown) power |= 0xC0 << 16;
        return power | (0x01 << 8);
    }

    static byte SpeedByte(string speed) => speed switch
    {
        "Slow" => 0xE1,
        "Fast" => 0xF5,
        _ => 0xEB,
    };

    static byte DirectionByte(string direction) => direction switch
    {
        "Left" => 1,
        "Up" => 2,
        "Down" => 3,
        _ => 0,
    };

    static string NormalizeSpeed(string speed) => speed switch
    {
        "Slow" => "Slow",
        "Fast" => "Fast",
        _ => "Normal",
    };

    static string NormalizeDirection(string direction) => direction switch
    {
        "Left" => "Left",
        "Up" => "Up",
        "Down" => "Down",
        _ => "Right",
    };

    static void Rgb(int[] color, out byte r, out byte g, out byte b)
    {
        r = color.Length > 0 ? (byte)Math.Clamp(color[0], 0, 255) : (byte)0;
        g = color.Length > 1 ? (byte)Math.Clamp(color[1], 0, 255) : (byte)0;
        b = color.Length > 2 ? (byte)Math.Clamp(color[2], 0, 255) : (byte)0;
    }

    bool WriteAura(params byte[][] messages)
    {
        var devices = FindDevices(AuraId);
        if (devices.Count == 0)
        {
            usbAura = false;
            return false;
        }

        bool wrote = false;
        lock (hidLock)
        {
            foreach (var device in devices)
            {
                try
                {
                    using var stream = device.Open();
                    foreach (var message in messages)
                    {
                        stream.Write(message);
                    }
                    wrote = true;
                }
                catch
                {
                    // Another process may hold the lighting device.
                }
            }
        }
        return wrote;
    }

    bool WriteInput(byte[] data)
    {
        var devices = FindDevices(InputId);
        if (devices.Count == 0) return false;

        bool wrote = false;
        lock (hidLock)
        {
            foreach (var device in devices)
            {
                try
                {
                    using var stream = device.Open();
                    int length = Math.Max(device.GetMaxFeatureReportLength(), data.Length);
                    var payload = new byte[length];
                    Array.Copy(data, payload, data.Length);
                    stream.SetFeature(payload);
                    wrote = true;
                }
                catch
                {
                    // Try the next matching ASUS HID device.
                }
            }
        }
        return wrote;
    }

    static List<HidDevice> FindDevices(byte reportId)
    {
        var found = new List<HidDevice>();
        try
        {
            foreach (var device in DeviceList.Local.GetHidDevices(AsusVendorId))
            {
                try
                {
                    if (!AuraPids.Contains(device.ProductID) || !device.CanOpen) continue;
                    if (device.GetMaxFeatureReportLength() <= 0) continue;
                    if (!device.GetReportDescriptor().TryGetReport(ReportType.Feature, reportId, out _)) continue;
                    found.Add(device);
                }
                catch
                {
                    // A device without a usable descriptor is not ours.
                }
            }
        }
        catch
        {
            // The HID service is unavailable.
        }
        return found;
    }

    void LoadState()
    {
        try
        {
            if (!File.Exists(statePath)) return;
            var saved = JsonSerializer.Deserialize<SavedState>(File.ReadAllText(statePath));
            if (saved is not null) state = saved;
        }
        catch
        {
            // A corrupt state file falls back to the defaults.
        }
    }

    void SaveState()
    {
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(statePath)!);
            File.WriteAllText(statePath, JsonSerializer.Serialize(state));
        }
        catch
        {
            // Lighting must still work when the profile is read-only.
        }
    }

    static readonly object hidLock = new();

    sealed class SavedState
    {
        public uint Mode { get; set; }
        public uint Zone { get; set; }
        public uint Brightness { get; set; } = 3;
        public byte Red1 { get; set; } = 255;
        public byte Green1 { get; set; } = 255;
        public byte Blue1 { get; set; } = 255;
        public byte Red2 { get; set; }
        public byte Green2 { get; set; }
        public byte Blue2 { get; set; }
        public string Speed { get; set; } = "Normal";
        public string Direction { get; set; } = "Right";
        public bool Boot { get; set; } = true;
        public bool Awake { get; set; } = true;
        public bool Sleep { get; set; } = true;
        public bool Shutdown { get; set; } = true;
    }
}

public sealed class AuraSnapshot
{
    public bool Supported { get; set; }
    public List<uint> Modes { get; set; } = [];
    public uint Brightness { get; set; }
    public List<uint> Brightnesses { get; set; } = [];
    public AuraModeSnapshot Mode { get; set; } = new();
    public List<AuraPowerSnapshot> Power { get; set; } = [];
    public List<uint> PowerZones { get; set; } = [];
    public List<uint> Zones { get; set; } = [];
    public uint DeviceType { get; set; }
    public string Path { get; set; } = "";
}

public sealed class AuraModeSnapshot
{
    public uint Mode { get; set; }
    public uint Zone { get; set; }
    public int[] Color1 { get; set; } = [0, 0, 0];
    public int[] Color2 { get; set; } = [0, 0, 0];
    public string Speed { get; set; } = "Normal";
    public string Direction { get; set; } = "Right";
}

public sealed class AuraPowerSnapshot
{
    public uint Zone { get; set; }
    public bool Boot { get; set; }
    public bool Awake { get; set; }
    public bool Sleep { get; set; }
    public bool Shutdown { get; set; }
}
