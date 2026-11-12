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
    static AuraController aura = null!;
    static FanStore fans = null!;
    static PerformanceProfiles profiles = null!;
    static SystemDetails systemInfo = null!;
    static string model = "";

    /// <summary>pendingGpu is the graphics mode waiting for a restart, kept
    /// with the profile so it survives a restart of the bridge.</summary>
    static string pendingGpu
    {
        get => profiles.PendingGpu;
        set => profiles.PendingGpu = value;
    }

    static int Main()
    {
        model = ReadModel();
        PowerLimits.Apply(model);
        acpi = new AsusACPI(model);
        aura = new AuraController(acpi, model);
        profiles = new PerformanceProfiles(acpi, StatePath());
        fans = new FanStore(acpi, profiles);
        systemInfo = ReadSystemInfo();
        fans.Load();
        profiles.Restore();
        fans.ApplyEnabled(PerformanceProfiles.Name(profiles.Current));
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

    /// <summary>StatePath is where the bridge keeps the profile it applied,
    /// beside the app's own settings.</summary>
    static string StatePath() => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
        "zephyr-center", "bridge.json");

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
                return SetProfile(r.Value);
            case "set_charge_limit":
                return SetChargeLimit(r.Value);
            case "set_feature":
                return SetFeature(r.Name ?? "", r.Value);
            case "set_fan_curve":
                return SetCurve(r);
            case "set_fan_curves_enabled":
                return SetCurvesEnabled(r);
            case "reset_fan_curves":
                return ResetCurves(r);
            case "set_gpu_mode":
                return SetGpuMode(r.Value);
            case "set_aura":
                return ApplyAura(r);
            case "set_aura_brightness":
                return ApplyAuraBrightness(r.Value);
            case "set_aura_power":
                return ApplyAuraPower(r);
            default:
                return new Response { Ok = false, Error = $"unknown op {r.Op}" };
        }
    }

    static Response SetProfile(int value)
    {
        if (!profiles.Apply(value))
            return new Response { Ok = false, Error = "the firmware refused the profile" };
        fans.ApplyEnabled(ProfileName(value));
        return new Response { Ok = true };
    }

    static Response SetChargeLimit(int value)
    {
        if (value < 20 || value > 100)
            return new Response { Ok = false, Error = "the charge limit must be between 20 and 100" };
        if (acpi.DeviceSet(AsusACPI.BatteryLimit, value) != 1)
            return new Response { Ok = false, Error = "the firmware refused the charge limit" };
        profiles.RememberChargeLimit(value);
        return new Response { Ok = true };
    }

    static Response Apply(int status) => status >= 0
        ? new Response { Ok = true }
        : new Response { Ok = false, Error = $"the firmware refused the write ({status})" };

    static string ProfileName(int code) => PerformanceProfiles.Name(code);

    static string ProfileName(string? profile) => Profiles.Normalize(profile);

    static string NormalizeFan(string? fan) => Profiles.NormalizeFan(fan);

    static Response SetFeature(string name, int value)
    {
        switch (name)
        {
            case "cpu_boost":
                return PowerSettings.SetCpuBoost(value)
                    ? new Response { Ok = true }
                    : new Response { Ok = false, Error = "the power plan refused the boost mode" };
            case "power_mode":
                return PowerSettings.SetPowerMode(value)
                    ? new Response { Ok = true }
                    : new Response { Ok = false, Error = "the power plan refused the power mode" };
            case "ppt_crossload":
                bool cross = false;
                if (acpi.IsSupported((uint)AsusACPI.PPT_CROSS9F))
                    cross |= acpi.DeviceSet((uint)AsusACPI.PPT_CROSS9F, value) >= 0;
                if (acpi.IsSupported((uint)AsusACPI.PPT_TOTAL9A))
                    cross |= acpi.DeviceSet((uint)AsusACPI.PPT_TOTAL9A, value) >= 0;
                return cross
                    ? new Response { Ok = true }
                    : new Response { Ok = false, Error = "the firmware refused the cross-loading limit" };
        }
        int endpoint = name switch
        {
            "panel_overdrive" => (int)AsusACPI.ScreenOverdrive,
            "boot_sound" => AsusACPI.BootSound,
            "ppt_pl1_spl" => AsusACPI.PPT_APUA3,
            "ppt_pl2_sppt" => AsusACPI.PPT_APUA0,
            "ppt_fppt" => AsusACPI.PPT_APUC1,
            "ppt_cpu" => AsusACPI.PPT_CPUB0,
            "ppt_gpu2cpu" => AsusACPI.PPT_GPUCPU9C,
            "ppt_cpu_temp" => AsusACPI.PPT_TEMP9E,
            "gpu_power" => AsusACPI.GPU_POWER,
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
        var curve = new FanCurve
        {
            Fan = NormalizeFan(r.Fan),
            Temp = [.. r.Temp],
            Pwm = [.. r.Pwm],
            Enabled = r.Enabled,
        };
        string? error = fans.Set(ProfileName(r.Profile), curve);
        return error is null
            ? new Response { Ok = true }
            : new Response { Ok = false, Error = error };
    }

    static Response SetCurvesEnabled(Request r)
    {
        string profile = ProfileName(r.Profile);
        string? error = fans.SetEnabled(profile, r.Enabled);
        return error is null
            ? new Response { Ok = true }
            : new Response { Ok = false, Error = error };
    }

    static Response ResetCurves(Request r)
    {
        string profile = ProfileName(r.Profile);
        string? error = fans.Reset(profile);
        return error is null
            ? new Response { Ok = true }
            : new Response { Ok = false, Error = error };
    }

    static Response ApplyAura(Request r)
    {
        if (r.Color1 is not { Length: 3 } || r.Color2 is not { Length: 3 })
            return new Response { Ok = false, Error = "lighting needs two RGB colours" };
        bool ok = aura.SetAura(r.Mode, r.Zone, r.Color1, r.Color2,
            r.Speed ?? "Normal", r.Direction ?? "Right");
        return ok
            ? new Response { Ok = true }
            : new Response { Ok = false, Error = "no Aura device accepted the write" };
    }

    static Response ApplyAuraBrightness(int value)
    {
        if (value < 0 || value > 3)
            return new Response { Ok = false, Error = "brightness must be between 0 and 3" };
        return aura.SetBrightness((uint)value)
            ? new Response { Ok = true }
            : new Response { Ok = false, Error = "no Aura device accepted the write" };
    }

    static Response ApplyAuraPower(Request r)
    {
        return aura.SetPower(r.Zone, r.Boot, r.Awake, r.Sleep, r.Shutdown)
            ? new Response { Ok = true }
            : new Response { Ok = false, Error = "no Aura device accepted the write" };
    }

    static Response SetGpuMode(int mode)
    {
        string target = mode switch
        {
            0 => "integrated",
            2 => "ultimate",
            _ => "hybrid",
        };
        var current = ReadGpuActual();
        if (!current.Supported)
            return new Response { Ok = false, Error = "this machine exposes no graphics switch" };
        // A MUX change is only applied at boot, so the mode the machine will
        // wake in counts as the one already chosen.
        string chosen = pendingGpu != "" ? pendingGpu : current.Mode;
        if (chosen == target)
        {
            return new Response { Ok = true };
        }

        // The MUX decides which GPU drives the panel, and the firmware only
        // moves it at boot. While it points at the discrete GPU the driver
        // refuses every write to the eco endpoint, so leaving Ultimate is a
        // MUX write alone: the eco flag follows on the next boot.
        if (current.Mux == 0)
        {
            if (target == "ultimate")
            {
                pendingGpu = "";
                return new Response { Ok = true };
            }
            if (acpi.DeviceSet(acpi.GPUMux, 1) != 1)
                return new Response { Ok = false, Error = "the firmware refused the graphics switch" };
            pendingGpu = target;
            return new Response { Ok = true };
        }

        // The MUX is away from the discrete GPU. Putting it back is another
        // reboot, and cancels a switch out of Ultimate that has not happened
        // yet.
        if (target == "ultimate")
        {
            if (!acpi.IsSupported(acpi.GPUMux))
                return new Response { Ok = false, Error = "this machine has no MUX to switch" };
            if (pendingGpu == "")
            {
                // The discrete GPU has to be awake before the panel can be
                // wired to it, so eco is cleared first and checked.
                if (current.Eco == 1 && acpi.DeviceSet(acpi.GPUEco, 0) != 1)
                    return new Response { Ok = false, Error = "the firmware would not wake the discrete GPU" };
                Thread.Sleep(500);
                if (acpi.IsSupported(acpi.GPUEco) && acpi.DeviceGet(acpi.GPUEco) == 1)
                    return new Response { Ok = false, Error = "the discrete GPU is still off" };
            }
            if (acpi.DeviceSet(acpi.GPUMux, 0) != 1)
                return new Response { Ok = false, Error = "the firmware refused the graphics switch" };
            pendingGpu = target;
            return new Response { Ok = true };
        }

        if (!acpi.IsSupported(acpi.GPUEco))
            return new Response { Ok = false, Error = "this machine cannot switch the discrete GPU off" };
        int eco = target == "integrated" ? 1 : 0;
        if (current.Eco != eco && acpi.DeviceSet(acpi.GPUEco, eco) != 1)
            return new Response { Ok = false, Error = "the firmware refused the graphics switch" };
        pendingGpu = "";
        return new Response { Ok = true };
    }

    static Response Snapshot()
    {
        var info = systemInfo;
        var s = new Snapshot
        {
            Connected = acpi.Connected,
            Model = info.Model,
            Board = info.Board,
            Bios = info.Bios,
            BiosDate = info.BiosDate,
            Cpu = info.Cpu,
            Igpu = info.Igpu,
            Dgpu = info.Dgpu,
            Kernel = info.Kernel,
            Os = info.Os,
            Profile = PerformanceProfiles.Name(profiles.Current),
            ProfileChoices = profiles.Choices(),
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
            Aura = aura.Snapshot(),
            ChargeSupported = acpi.IsSupported(AsusACPI.BatteryLimit),
        };
        if (s.ChargeSupported)
        {
            s.ChargeLimit = profiles.ChargeLimit;
        }
        if (acpi.IsSupported(AsusACPI.ScreenOverdrive))
        {
            s.Features["panel_overdrive"] = Feature(acpi, AsusACPI.ScreenOverdrive);
        }
        if (acpi.IsSupported((uint)AsusACPI.BootSound))
        {
            s.Features["boot_sound"] = Feature(acpi, (uint)AsusACPI.BootSound);
        }
        if (acpi.IsSupported(acpi.GPUEco))
        {
            s.Features["dgpu_disable"] = Feature(acpi, acpi.GPUEco);
        }
        if (acpi.IsSupported(acpi.GPUMux))
        {
            s.Features["gpu_mux_mode"] = Feature(acpi, acpi.GPUMux);
        }
        AddPowerFeatures(s);
        s.Curves = fans.Snapshot();
        return new Response { Ok = true, Snapshot = s };
    }

    /// <summary>AddPowerFeatures reports the power limits the way G-Helper's
    /// Fans+Power page does: an endpoint that exists is offered, with the
    /// model-adjusted ceiling, and a reading the firmware returns as zero
    /// falls back to a sensible default so the control still opens.</summary>
    static void AddPowerFeatures(Snapshot s)
    {
        bool a0 = acpi.IsSupported((uint)AsusACPI.PPT_APUA0);
        bool allAmd = acpi.IsSupported((uint)AsusACPI.PPT_CPUB0)
            && !acpi.IsSupported((uint)AsusACPI.PPT_GPUC0);

        if (a0)
        {
            s.Features["ppt_pl1_spl"] = RangeFeature(acpi, (uint)AsusACPI.PPT_APUA3,
                PowerLimits.MinTotal, PowerLimits.MaxTotal, PowerLimits.DefaultTotal);
            if (!allAmd)
                s.Features["ppt_pl2_sppt"] = RangeFeature(acpi, (uint)AsusACPI.PPT_APUA0,
                    PowerLimits.MinTotal, PowerLimits.MaxTotal, PowerLimits.DefaultTotal);
        }
        if (allAmd)
            s.Features["ppt_cpu"] = RangeFeature(acpi, (uint)AsusACPI.PPT_CPUB0,
                PowerLimits.MinCPU, PowerLimits.MaxCPU, PowerLimits.DefaultCPU);
        else if (acpi.IsSupported((uint)AsusACPI.PPT_APUC1))
            s.Features["ppt_fppt"] = RangeFeature(acpi, (uint)AsusACPI.PPT_APUC1,
                PowerLimits.MinTotal, PowerLimits.MaxTotal, PowerLimits.DefaultTotal);

        if (acpi.IsSupported((uint)AsusACPI.PPT_GPUC0))
            s.Features["nv_dynamic_boost"] = RangeFeature(acpi, (uint)AsusACPI.PPT_GPUC0,
                PowerLimits.MinGPUBoost, PowerLimits.MaxGPUBoost, PowerLimits.MaxGPUBoost);
        if (acpi.IsSupported((uint)AsusACPI.PPT_GPUC2))
            s.Features["nv_temp_target"] = RangeFeature(acpi, (uint)AsusACPI.PPT_GPUC2,
                PowerLimits.MinGPUTemp, PowerLimits.MaxGPUTemp, PowerLimits.MaxGPUTemp);

        // Advanced: the older Intel/NVIDIA endpoints and the GPU TGP.
        if (acpi.IsSupported((uint)AsusACPI.PPT_CROSS9F) || acpi.IsSupported((uint)AsusACPI.PPT_TOTAL9A))
        {
            uint crossEndpoint = acpi.IsSupported((uint)AsusACPI.PPT_CROSS9F)
                ? (uint)AsusACPI.PPT_CROSS9F : (uint)AsusACPI.PPT_TOTAL9A;
            s.Features["ppt_crossload"] = RangeFeature(acpi, crossEndpoint,
                PowerLimits.MinCrossLoad, PowerLimits.MaxCrossLoad, PowerLimits.MaxCrossLoad);
        }
        if (acpi.IsSupported((uint)AsusACPI.PPT_GPUCPU9C))
            s.Features["ppt_gpu2cpu"] = RangeFeature(acpi, (uint)AsusACPI.PPT_GPUCPU9C,
                PowerLimits.MinGPUtoCPU, PowerLimits.MaxGPUtoCPU, PowerLimits.MaxGPUtoCPU,
                PowerLimits.StepGPUtoCPU);
        if (acpi.IsSupported((uint)AsusACPI.PPT_TEMP9E))
            s.Features["ppt_cpu_temp"] = RangeFeature(acpi, (uint)AsusACPI.PPT_TEMP9E,
                PowerLimits.MinCPUTemp, PowerLimits.MaxCPUTemp, PowerLimits.MaxCPUTemp);
        int gpuBase = acpi.DeviceGet((uint)AsusACPI.GPU_BASE);
        if (gpuBase > 0 && acpi.IsSupported((uint)AsusACPI.GPU_POWER))
            s.Features["gpu_power"] = RangeFeature(acpi, (uint)AsusACPI.GPU_POWER,
                PowerLimits.MinGPUPower, PowerLimits.MaxGPUPower, PowerLimits.MaxGPUPower);

        // Advanced: the Windows power plan, as G-Helper's advanced tab.
        int boost = PowerSettings.GetCpuBoost();
        if (boost >= 0)
            s.Features["cpu_boost"] = new Feature
            {
                Supported = true,
                Value = boost,
                Values = [0, 1, 2, 3, 4],
            };
        int powerMode = PowerSettings.GetPowerMode();
        if (powerMode >= 0)
            s.Features["power_mode"] = new Feature
            {
                Supported = true,
                Value = powerMode,
                Values = [0, 1, 2, 3],
            };
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

    static Feature RangeFeature(AsusACPI acpi, uint endpoint, int min, int max, int fallback, int step = 1)
    {
        int value = acpi.DeviceGet(endpoint);
        if (value < min || value > max) value = fallback;
        return new Feature
        {
            Supported = true,
            Value = Math.Clamp(value, min, max),
            Min = min,
            Max = max,
            Step = step,
        };
    }

    static GpuState ReadGpu()
    {
        var gpu = ReadGpuActual();
        if (!gpu.Supported) return new GpuState();
        // The MUX has moved, so the queued switch has happened.
        if (pendingGpu != "" && gpu.Mode == pendingGpu) pendingGpu = "";
        return new GpuState
        {
            Supported = true,
            Mode = gpu.Mode,
            Modes = gpu.Modes,
            Pending = pendingGpu,
            Reboot = pendingGpu != "",
        };
    }

    static (bool Supported, string Mode, List<string> Modes, int Mux, int Eco) ReadGpuActual()
    {
        int eco = acpi.IsSupported(acpi.GPUEco) ? acpi.DeviceGet(acpi.GPUEco) : -1;
        int mux = acpi.IsSupported(acpi.GPUMux) ? acpi.DeviceGet(acpi.GPUMux) : -1;
        if (eco < 0 && mux < 0) return (false, "", [], mux, eco);
        string mode = mux == 0 ? "ultimate" : eco == 1 ? "integrated" : "hybrid";
        var modes = new List<string>();
        if (eco >= 0) modes.Add("integrated");
        modes.Add("hybrid");
        if (mux >= 0) modes.Add("ultimate");
        return (true, mode, modes, mux, eco);
    }

    static Battery ReadBattery()
    {
        var battery = new Battery();
        try
        {
            using var searcher = new ManagementObjectSearcher(
                "SELECT EstimatedChargeRemaining, BatteryStatus FROM Win32_Battery");
            foreach (var obj in searcher.Get())
            {
                using (obj)
                {
                    int status = Convert.ToInt32(obj["BatteryStatus"] ?? 0);
                    battery.Present = true;
                    battery.Capacity = Convert.ToInt32(obj["EstimatedChargeRemaining"] ?? 0);
                    battery.Status = status switch
                    {
                        3 => "Full",
                        6 or 7 or 8 or 9 => "Charging",
                        1 => "Discharging",
                        11 => "Not charging",
                        _ => "Unknown",
                    };
                }
            }
        }
        catch
        {
            // No battery, as on a desktop.
        }
        if (!battery.Present) return battery;

        EnsureBatteryStatic();
        long remaining = 0;
        long dischargeRate = 0, chargeRate = 0;
        long runtime = 0;
        bool powerOnline = false;
        battery.Cycles = batteryCycles;

        try
        {
            using var searcher = new ManagementObjectSearcher(@"root\wmi", "SELECT * FROM BatteryStatus");
            foreach (var obj in searcher.Get())
            {
                using (obj)
                {
                    dischargeRate = ToInt64(obj["DischargeRate"]);
                    chargeRate = ToInt64(obj["ChargeRate"]);
                    runtime = ToInt64(obj["EstimatedRuntime"]);
                    remaining = ToInt64(obj["RemainingCapacity"]);
                    powerOnline = Convert.ToBoolean(obj["PowerOnline"] ?? false);
                }
            }
        }
        catch
        {
            // BatteryStatus is absent on some firmware.
        }

        if (batteryFull > 0 && batteryDesign > 0)
            battery.Health = (double)batteryFull / batteryDesign * 100;

        if (dischargeRate > 0 && dischargeRate < 1_000_000)
            battery.PowerW = dischargeRate / 1000.0;
        else if (chargeRate > 0 && chargeRate < 1_000_000)
            battery.PowerW = chargeRate / 1000.0;

        if (battery.Capacity <= 0 && batteryFull > 0 && remaining > 0)
            battery.Capacity = (int)Math.Round((double)remaining / batteryFull * 100);

        if (battery.Status == "Unknown")
            battery.Status = powerOnline ? "Not charging" : "Discharging";
        if (powerOnline && battery.Status == "Discharging")
            battery.Status = "Not charging";

        if (runtime > 0 && runtime < 86400 && battery.Status == "Discharging")
        {
            long minutes = runtime / 60;
            battery.TimeLeft = $"{minutes / 60}h {minutes % 60}m";
        }

        return battery;
    }

    static long batteryDesign;
    static long batteryFull;
    static int batteryCycles;
    static bool batteryStaticRead;

    static void EnsureBatteryStatic()
    {
        if (batteryStaticRead) return;
        batteryStaticRead = true;
        batteryDesign = WmiUInt64(@"root\wmi",
            "SELECT DesignedCapacity FROM BatteryStaticData", "DesignedCapacity");
        batteryCycles = (int)WmiUInt64(@"root\wmi",
            "SELECT CycleCount FROM BatteryStaticData", "CycleCount");
        batteryFull = WmiUInt64(@"root\wmi",
            "SELECT FullChargedCapacity FROM BatteryFullChargedCapacity", "FullChargedCapacity");
    }

    static long WmiUInt64(string scope, string query, string property)
    {
        try
        {
            using var searcher = new ManagementObjectSearcher(scope, query);
            foreach (var obj in searcher.Get())
            {
                using (obj) return ToInt64(obj[property]);
            }
        }
        catch
        {
            // The firmware does not expose this class.
        }
        return 0;
    }

    static long ToInt64(object? value)
    {
        try
        {
            return value is null ? 0 : Convert.ToInt64(value);
        }
        catch
        {
            return 0;
        }
    }

    static SystemDetails ReadSystemInfo()
    {
        var info = new SystemDetails
        {
            Model = model,
            Board = WmiString("Win32_BaseBoard", "Product"),
            Bios = WmiString("Win32_BIOS", "SMBIOSBIOSVersion"),
            BiosDate = FormatBiosDate(WmiString("Win32_BIOS", "ReleaseDate")),
            Cpu = ReadCpu(),
            Kernel = WmiString("Win32_OperatingSystem", "BuildNumber"),
            Os = WmiString("Win32_OperatingSystem", "Caption"),
        };
        ReadGpuNames(out string igpu, out string dgpu);
        info.Igpu = igpu;
        info.Dgpu = dgpu;
        return info;
    }

    static string WmiString(string className, string property)
    {
        try
        {
            using var searcher = new ManagementObjectSearcher($"SELECT {property} FROM {className}");
            foreach (var obj in searcher.Get())
            {
                using (obj) return obj[property]?.ToString()?.Trim() ?? "";
            }
        }
        catch
        {
            // WMI is unavailable.
        }
        return "";
    }

    static void ReadGpuNames(out string integrated, out string discrete)
    {
        integrated = "";
        discrete = "";
        try
        {
            using var searcher = new ManagementObjectSearcher("SELECT Name, PNPDeviceID FROM Win32_VideoController");
            foreach (var obj in searcher.Get())
            {
                using (obj)
                {
                    string name = obj["Name"]?.ToString()?.Trim() ?? "";
                    string pnp = obj["PNPDeviceID"]?.ToString() ?? "";
                    if (name == "") continue;
                    string lower = name.ToLowerInvariant();
                    bool nvidia = lower.Contains("nvidia") || pnp.Contains("VEN_10DE");
                    bool intel = lower.Contains("intel") || pnp.Contains("VEN_8086");
                    bool amdDiscrete = lower.Contains("rx ") || lower.Contains("radeon pro")
                        || lower.Contains("firepro");
                    if (nvidia || amdDiscrete)
                    {
                        if (discrete == "") discrete = name;
                    }
                    else if (intel || lower.Contains("radeon"))
                    {
                        if (integrated == "") integrated = name;
                    }
                    else if (integrated == "")
                    {
                        integrated = name;
                    }
                    else if (discrete == "")
                    {
                        discrete = name;
                    }
                }
            }
        }
        catch
        {
            // WMI is unavailable.
        }
    }

    static string FormatBiosDate(string value)
    {
        if (value.Length >= 8 && DateTime.TryParseExact(value[..8], "yyyyMMdd",
                System.Globalization.CultureInfo.InvariantCulture,
                System.Globalization.DateTimeStyles.None, out var date))
        {
            return date.ToString("yyyy-MM-dd");
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

sealed class FanStore
{
    static readonly (string Name, int Code)[] Profiles =
    [
        ("balanced", 0), ("performance", 1), ("quiet", 2),
    ];

    static readonly (string Name, AsusFan Fan)[] Fans =
    [
        ("CPU", AsusFan.CPU), ("GPU", AsusFan.GPU), ("MID", AsusFan.Mid),
    ];

    readonly AsusACPI acpi;
    readonly PerformanceProfiles profiles;
    readonly Dictionary<string, Dictionary<string, FanCurve>> curves = [];
    readonly Dictionary<string, Dictionary<string, FanCurve>> defaults = [];
    readonly HashSet<string> enabled = [];

    public FanStore(AsusACPI acpi, PerformanceProfiles profiles)
    {
        this.acpi = acpi;
        this.profiles = profiles;
    }

    public void Load()
    {
        foreach (var (profile, code) in Profiles)
        {
            curves[profile] = [];
            defaults[profile] = [];
            foreach (var (name, fan) in Fans)
            {
                byte[] raw;
                try
                {
                    raw = acpi.GetFanCurve(fan, code);
                }
                catch
                {
                    continue;
                }
                var curve = FromFirmware(name, raw);
                if (curve is null) continue;
                curves[profile][name] = Clone(curve);
                defaults[profile][name] = Clone(curve);
            }
        }
    }

    public Dictionary<string, List<FanCurve>> Snapshot()
    {
        var all = new Dictionary<string, List<FanCurve>>();
        foreach (var (profile, _) in Profiles)
        {
            var list = new List<FanCurve>();
            foreach (var (name, _) in Fans)
            {
                if (!curves[profile].TryGetValue(name, out var curve)) continue;
                var copy = Clone(curve);
                copy.Enabled = enabled.Contains(profile);
                list.Add(copy);
            }
            all[profile] = list;
        }
        return all;
    }

    public string? Set(string profile, FanCurve curve)
    {
        if (!curves.TryGetValue(profile, out var byFan))
            return $"unknown profile {profile}";
        if (!byFan.ContainsKey(curve.Fan))
            return $"fan {curve.Fan} is not present on this machine";
        byFan[curve.Fan] = Clone(curve);
        if (enabled.Contains(profile))
            return Write(profile, curve);
        return null;
    }

    public string? SetEnabled(string profile, bool on)
    {
        if (!curves.TryGetValue(profile, out var byFan))
            return $"unknown profile {profile}";
        if (on)
        {
            enabled.Add(profile);
            return Apply(profile);
        }
        enabled.Remove(profile);
        return RestoreDefaults(profile);
    }

    public void ApplyEnabled(string profile)
    {
        if (enabled.Contains(profile))
            Apply(profile);
    }

    public string? Reset(string profile)
    {
        if (!curves.TryGetValue(profile, out var byFan) || !defaults.TryGetValue(profile, out var factory))
            return $"unknown profile {profile}";
        string? failure = null;
        foreach (var (name, curve) in factory)
        {
            var copy = Clone(curve);
            byFan[name] = copy;
            failure ??= Write(profile, copy);
        }
        return failure;
    }

    string? Apply(string profile)
    {
        if (!curves.TryGetValue(profile, out var byFan)) return null;
        foreach (var curve in byFan.Values)
        {
            string? error = Write(profile, curve);
            if (error is not null) return error;
        }
        return null;
    }

    string? RestoreDefaults(string profile)
    {
        if (!defaults.TryGetValue(profile, out var byFan)) return null;
        foreach (var curve in byFan.Values)
        {
            string? error = Write(profile, curve);
            if (error is not null) return error;
        }
        return null;
    }

    string? Write(string profile, FanCurve curve)
    {
        int target = PerformanceProfiles.Code(profile);
        int current = profiles.Current;
        bool switched = current != target;
        if (switched)
        {
            profiles.Apply(target);
            Thread.Sleep(80);
        }

        int status;
        try
        {
            status = acpi.SetFanCurve(FanOf(curve.Fan), ToFirmware(curve));
        }
        catch (Exception e)
        {
            status = -1;
            if (switched) profiles.Apply(current);
            return e.Message;
        }
        if (switched) profiles.Apply(current);
        return status == 1 ? null : $"the firmware refused the {curve.Fan} curve ({status})";
    }

    static FanCurve Clone(FanCurve c) => new()
    {
        Fan = c.Fan,
        Temp = [.. c.Temp],
        Pwm = [.. c.Pwm],
        Enabled = c.Enabled,
    };

    static FanCurve? FromFirmware(string name, byte[] raw)
    {
        if (raw.Length != 16 || raw.All(b => b == 0)) return null;
        var curve = new FanCurve { Fan = name, Temp = new int[8], Pwm = new int[8] };
        for (int i = 0; i < 8; i++)
        {
            curve.Temp[i] = Math.Clamp((int)raw[i], 0, 100);
            curve.Pwm[i] = Math.Clamp((int)raw[i + 8], 0, 100) * 255 / 100;
        }
        return curve;
    }

    static byte[] ToFirmware(FanCurve curve)
    {
        var raw = new byte[16];
        for (int i = 0; i < 8; i++)
        {
            raw[i] = (byte)Math.Clamp(curve.Temp[i], 0, 100);
            raw[i + 8] = (byte)(Math.Clamp(curve.Pwm[i], 0, 255) * 100 / 255);
        }
        return raw;
    }

    static AsusFan FanOf(string name) => name.ToUpperInvariant() switch
    {
        "GPU" => AsusFan.GPU,
        "MID" => AsusFan.Mid,
        _ => AsusFan.CPU,
    };

}

static class Profiles
{
    public static string Normalize(string? profile) => profile switch
    {
        "performance" => "performance",
        "quiet" => "quiet",
        "balanced" => "balanced",
        _ => "balanced",
    };

    public static string NormalizeFan(string? fan) => fan?.ToUpperInvariant() switch
    {
        "GPU" => "GPU",
        "MID" => "MID",
        _ => "CPU",
    };
}

sealed class Request
{
    public string Op { get; set; } = "";
    public string? Name { get; set; }
    public int Value { get; set; }
    public string? Profile { get; set; }
    public string? Fan { get; set; }
    public int[]? Temp { get; set; }
    public int[]? Pwm { get; set; }
    public bool Enabled { get; set; }
    public uint Mode { get; set; }
    public uint Zone { get; set; }
    public int[]? Color1 { get; set; }
    public int[]? Color2 { get; set; }
    public string? Speed { get; set; }
    public string? Direction { get; set; }
    public bool Boot { get; set; }
    public bool Awake { get; set; }
    public bool Sleep { get; set; }
    public bool Shutdown { get; set; }
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
    public string Board { get; set; } = "";
    public string Bios { get; set; } = "";
    public string BiosDate { get; set; } = "";
    public string Cpu { get; set; } = "";
    public string Igpu { get; set; } = "";
    public string Dgpu { get; set; } = "";
    public string Kernel { get; set; } = "";
    public string Os { get; set; } = "";
    public string Profile { get; set; } = "balanced";
    public List<string> ProfileChoices { get; set; } = [];
    public int ChargeLimit { get; set; }
    public bool ChargeSupported { get; set; }
    public Sensors Sensors { get; set; } = new();
    public Battery Battery { get; set; } = new();
    public GpuState Gpu { get; set; } = new();
    public AuraSnapshot Aura { get; set; } = new();
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
    public string TimeLeft { get; set; } = "";
}

sealed class GpuState
{
    public bool Supported { get; set; }
    public string Mode { get; set; } = "";
    public List<string> Modes { get; set; } = [];
    public string Pending { get; set; } = "";
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
    public int[] Temp { get; set; } = [];
    public int[] Pwm { get; set; } = [];
    public bool Enabled { get; set; }
}

sealed class SystemDetails
{
    public string Model { get; set; } = "";
    public string Board { get; set; } = "";
    public string Bios { get; set; } = "";
    public string BiosDate { get; set; } = "";
    public string Cpu { get; set; } = "";
    public string Igpu { get; set; } = "";
    public string Dgpu { get; set; } = "";
    public string Kernel { get; set; } = "";
    public string Os { get; set; } = "";
}
