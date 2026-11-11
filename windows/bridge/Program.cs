// SPDX-License-Identifier: GPL-3.0-or-later

using System.Management;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace ZephyrBridge;

/// <summary>
/// Program answers one JSON request per line on stdin with one JSON reply
/// per line on stdout, so the Go app can drive G-Helper's ACPI layer
/// without the WinForms UI.
/// </summary>
static class Program
{
    static readonly JsonSerializerOptions Json = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    static AsusACPI acpi = null!;
    static string model = "";

    static int Main()
    {
        model = ReadModel();
        acpi = new AsusACPI(model);
        using var input = new StreamReader(Console.OpenStandardInput());
        using var output = new StreamWriter(Console.OpenStandardOutput()) { AutoFlush = true };
        string? line;
        while ((line = input.ReadLine()) != null)
        {
            Response response;
            try
            {
                var request = JsonSerializer.Deserialize<Request>(line, Json) ?? new Request();
                response = Handle(request);
            }
            catch (Exception e)
            {
                response = new Response { Ok = false, Error = e.Message };
            }
            output.WriteLine(JsonSerializer.Serialize(response, Json));
        }
        acpi.Close();
        return 0;
    }

    /// <summary>ReadModel reads the SMBIOS model name, as G-Helper's AppConfig does.</summary>
    static string ReadModel()
    {
        try
        {
            using var searcher = new ManagementObjectSearcher("Select * from Win32_ComputerSystem");
            foreach (var obj in searcher.Get())
            {
                using (obj) return obj["Model"]?.ToString() ?? "";
            }
        }
        catch
        {
            // Not on ASUS hardware, or WMI is unavailable.
        }
        return "";
    }

    static Response Handle(Request r)
    {
        switch (r.Op)
        {
            case "hello":
                return new Response
                {
                    Ok = true,
                    Backend = "ghelper",
                    Version = "0.1.0",
                    DaemonVersion = typeof(AsusACPI).Assembly.GetName().Version?.ToString() ?? "",
                };
            case "snapshot":
                return Snapshot();
            case "set_profile":
                return Apply(acpi.DeviceSet(AsusACPI.PerformanceMode, r.Value));
            case "set_charge_limit":
                return Apply(acpi.DeviceSet(AsusACPI.BatteryLimit, r.Value));
            case "set_feature":
                return SetFeature(r.Name ?? "", r.Value);
            case "set_fan_curve":
                return SetCurve(r);
            case "set_gpu_mode":
                return SetGpuMode(r.Value);
            default:
                return new Response { Ok = false, Error = $"unknown op {r.Op}" };
        }
    }

    static Response Apply(int status) => status >= 0
        ? new Response { Ok = true }
        : new Response { Ok = false, Error = $"the firmware refused the write ({status})" };

    static Response SetFeature(string name, int value)
    {
        int endpoint = name switch
        {
            "panel_overdrive" => (int)AsusACPI.ScreenOverdrive,
            "boot_sound" => AsusACPI.BootSound,
            "ppt_pl1_spl" => AsusACPI.PPT_APUA3,
            "ppt_pl2_sppt" => AsusACPI.PPT_APUA0,
            "nv_dynamic_boost" => AsusACPI.PPT_GPUC0,
            "nv_temp_target" => AsusACPI.PPT_GPUC2,
            _ => -1,
        };
        if (endpoint < 0) return new Response { Ok = false, Error = $"no feature {name}" };
        return Apply(acpi.DeviceSet((uint)endpoint, value));
    }

    static Response SetCurve(Request r)
    {
        if (r.Temp is not { Length: 8 } || r.Pwm is not { Length: 8 })
            return new Response { Ok = false, Error = "a fan curve needs 8 temperatures and 8 speeds" };
        var curve = new byte[16];
        for (int i = 0; i < 8; i++)
        {
            curve[i] = (byte)r.Temp[i];
            curve[i + 8] = (byte)r.Pwm[i];
        }
        var fan = r.Fan?.ToUpperInvariant() switch
        {
            "GPU" => AsusFan.GPU,
            "MID" => AsusFan.Mid,
            _ => AsusFan.CPU,
        };
        return Apply(acpi.SetFanCurve(fan, curve));
    }

    static Response SetGpuMode(int mode)
    {
        // 0 Integrated, 1 Hybrid, 2 Ultimate, as G-Helper's GPUModeControl.
        int eco = mode == 0 ? 1 : 0;
        int mux = mode == 2 ? 0 : 1;
        int status = 0;
        if (acpi.IsSupported(acpi.GPUEco)) status = acpi.DeviceSet(acpi.GPUEco, eco);
        if (acpi.IsSupported(acpi.GPUMux)) status = acpi.DeviceSet(acpi.GPUMux, mux);
        return Apply(status);
    }

    static Response Snapshot()
    {
        var s = new Snapshot
        {
            Connected = acpi.Connected,
            Model = model,
            Bios = ReadBios(),
            Cpu = ReadCpu(),
            Profile = acpi.DeviceGet(AsusACPI.PerformanceMode) switch
            {
                AsusACPI.PerformanceTurbo => "performance",
                AsusACPI.PerformanceSilent => "quiet",
                _ => "balanced",
            },
            ProfileChoices = ["quiet", "balanced", "performance"],
            Sensors = new Sensors
            {
                CpuTemp = acpi.DeviceGet((uint)AsusACPI.Temp_CPU),
                GpuTemp = acpi.DeviceGet((uint)AsusACPI.Temp_GPU),
                CpuRpm = Math.Max(0, acpi.GetFan(AsusFan.CPU)),
                GpuRpm = Math.Max(0, acpi.GetFan(AsusFan.GPU)),
                MidRpm = Math.Max(0, acpi.GetFan(AsusFan.Mid)),
            },
            Battery = ReadBattery(),
            Gpu = ReadGpu(),
            ChargeSupported = acpi.IsSupported(AsusACPI.BatteryLimit),
        };
        if (s.ChargeSupported)
        {
            s.ChargeLimit = acpi.DeviceGet(AsusACPI.BatteryLimit);
        }
        if (acpi.IsSupported(AsusACPI.ScreenOverdrive))
        {
            s.Features["panel_overdrive"] = Feature(acpi, AsusACPI.ScreenOverdrive);
        }
        if (acpi.IsSupported((uint)AsusACPI.BootSound))
        {
            s.Features["boot_sound"] = Feature(acpi, (uint)AsusACPI.BootSound);
        }
        s.Features["ppt_pl1_spl"] = RangeFeature(acpi, (uint)AsusACPI.PPT_APUA3, 25, 90);
        s.Features["ppt_pl2_sppt"] = RangeFeature(acpi, (uint)AsusACPI.PPT_APUA0, 25, 135);
        s.Features["nv_dynamic_boost"] = RangeFeature(acpi, (uint)AsusACPI.PPT_GPUC0, 5, 25);
        s.Features["nv_temp_target"] = RangeFeature(acpi, (uint)AsusACPI.PPT_GPUC2, 75, 87);
        s.Curves = ReadCurves();
        return new Response { Ok = true, Snapshot = s };
    }

    static Feature Feature(AsusACPI acpi, uint endpoint)
    {
        int value = acpi.DeviceGet(endpoint);
        return new Feature
        {
            Supported = value >= 0,
            Value = value,
            Values = [0, 1],
        };
    }

    static Feature RangeFeature(AsusACPI acpi, uint endpoint, int min, int max)
    {
        int value = acpi.DeviceGet(endpoint);
        return new Feature
        {
            Supported = value >= 0,
            Value = value,
            Min = min,
            Max = max,
            Step = 1,
        };
    }

    static Dictionary<string, List<FanCurve>> ReadCurves()
    {
        var modes = new (string Name, int Code)[] { ("balanced", 0), ("performance", 1), ("quiet", 2) };
        var fans = new (string Name, AsusFan Fan)[] { ("CPU", AsusFan.CPU), ("GPU", AsusFan.GPU), ("MID", AsusFan.Mid) };
        var all = new Dictionary<string, List<FanCurve>>();
        foreach (var (name, code) in modes)
        {
            var curves = new List<FanCurve>();
            foreach (var (fanName, fan) in fans)
            {
                var raw = acpi.GetFanCurve(fan, code);
                if (raw.Length != 16 || raw.All(b => b == 0)) continue;
                curves.Add(new FanCurve
                {
                    Fan = fanName,
                    Temp = raw[..8],
                    Pwm = raw[8..16],
                    Enabled = false,
                });
            }
            all[name] = curves;
        }
        return all;
    }

    static GpuState ReadGpu()
    {
        int eco = acpi.IsSupported(acpi.GPUEco) ? acpi.DeviceGet(acpi.GPUEco) : -1;
        int mux = acpi.IsSupported(acpi.GPUMux) ? acpi.DeviceGet(acpi.GPUMux) : -1;
        if (eco < 0 && mux < 0) return new GpuState();
        string mode = mux == 0 ? "ultimate" : eco == 1 ? "integrated" : "hybrid";
        var modes = new List<string>();
        if (eco >= 0) modes.Add("integrated");
        modes.Add("hybrid");
        if (mux >= 0) modes.Add("ultimate");
        return new GpuState { Supported = true, Mode = mode, Modes = modes };
    }

    static Battery ReadBattery()
    {
        try
        {
            using var searcher = new ManagementObjectSearcher(
                "SELECT EstimatedChargeRemaining, BatteryStatus FROM Win32_Battery");
            foreach (var obj in searcher.Get())
            {
                using (obj)
                {
                    int status = Convert.ToInt32(obj["BatteryStatus"] ?? 0);
                    return new Battery
                    {
                        Present = true,
                        Capacity = Convert.ToInt32(obj["EstimatedChargeRemaining"] ?? 0),
                        Status = status switch
                        {
                            2 => "Charging",
                            1 => "Discharging",
                            3 => "Full",
                            _ => "Unknown",
                        },
                    };
                }
            }
        }
        catch
        {
            // No battery, as on a desktop.
        }
        return new Battery();
    }

    static string ReadBios()
    {
        try
        {
            using var searcher = new ManagementObjectSearcher("SELECT SMBIOSBIOSVersion FROM Win32_BIOS");
            foreach (var obj in searcher.Get())
            {
                using (obj) return obj["SMBIOSBIOSVersion"]?.ToString() ?? "";
            }
        }
        catch
        {
            // WMI is unavailable.
        }
        return "";
    }

    static string ReadCpu()
    {
        try
        {
            using var key = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(
                @"HARDWARE\DESCRIPTION\System\CentralProcessor\0");
            return key?.GetValue("ProcessorNameString")?.ToString()?.Trim() ?? "";
        }
        catch
        {
            return "";
        }
    }
}

sealed class Request
{
    public string Op { get; set; } = "";
    public string? Name { get; set; }
    public int Value { get; set; }
    public string? Fan { get; set; }
    public int[]? Temp { get; set; }
    public int[]? Pwm { get; set; }
}

sealed class Response
{
    public bool Ok { get; set; }
    public string? Error { get; set; }
    public string? Backend { get; set; }
    public string? Version { get; set; }
    public string? DaemonVersion { get; set; }
    public Snapshot? Snapshot { get; set; }
}

sealed class Snapshot
{
    public bool Connected { get; set; }
    public string Model { get; set; } = "";
    public string Bios { get; set; } = "";
    public string Cpu { get; set; } = "";
    public string Profile { get; set; } = "balanced";
    public List<string> ProfileChoices { get; set; } = [];
    public int ChargeLimit { get; set; }
    public bool ChargeSupported { get; set; }
    public Sensors Sensors { get; set; } = new();
    public Battery Battery { get; set; } = new();
    public GpuState Gpu { get; set; } = new();
    public Dictionary<string, Feature> Features { get; set; } = [];
    public Dictionary<string, List<FanCurve>> Curves { get; set; } = [];
}

sealed class Sensors
{
    public double CpuTemp { get; set; }
    public double GpuTemp { get; set; }
    public int CpuRpm { get; set; }
    public int GpuRpm { get; set; }
    public int MidRpm { get; set; }
}

sealed class Battery
{
    public bool Present { get; set; }
    public int Capacity { get; set; }
    public string Status { get; set; } = "";
    public double PowerW { get; set; }
    public double Health { get; set; }
    public int Cycles { get; set; }
}

sealed class GpuState
{
    public bool Supported { get; set; }
    public string Mode { get; set; } = "";
    public List<string> Modes { get; set; } = [];
    public bool Reboot { get; set; }
}

sealed class Feature
{
    public bool Supported { get; set; }
    public int Value { get; set; }
    public int Min { get; set; }
    public int Max { get; set; }
    public int Step { get; set; }
    public List<int> Values { get; set; } = [];
}

sealed class FanCurve
{
    public string Fan { get; set; } = "";
    public byte[] Temp { get; set; } = [];
    public byte[] Pwm { get; set; } = [];
    public bool Enabled { get; set; }
}
