// SPDX-License-Identifier: GPL-3.0-or-later
//
// Derived from G-Helper's app/Mode/PowerNative.cs
// (https://github.com/seerge/g-helper, GPL-3.0-or-later): the Windows
// power-plan controls the Fans+Power page exposes — the CPU boost mode
// and the power-mode overlay slider. Trimmed to those two.

using System.Runtime.InteropServices;

namespace ZephyrBridge;

/// <summary>PowerSettings reads and writes the active Windows power plan.</summary>
static class PowerSettings
{
    [DllImport("powrprof.dll", CharSet = CharSet.Unicode)]
    static extern uint PowerWriteACValueIndex(IntPtr root,
        [MarshalAs(UnmanagedType.LPStruct)] Guid scheme,
        [MarshalAs(UnmanagedType.LPStruct)] Guid subgroup,
        [MarshalAs(UnmanagedType.LPStruct)] Guid setting, int value);

    [DllImport("powrprof.dll", CharSet = CharSet.Unicode)]
    static extern uint PowerWriteDCValueIndex(IntPtr root,
        [MarshalAs(UnmanagedType.LPStruct)] Guid scheme,
        [MarshalAs(UnmanagedType.LPStruct)] Guid subgroup,
        [MarshalAs(UnmanagedType.LPStruct)] Guid setting, int value);

    [DllImport("powrprof.dll", CharSet = CharSet.Unicode)]
    static extern uint PowerReadACValueIndex(IntPtr root,
        [MarshalAs(UnmanagedType.LPStruct)] Guid scheme,
        [MarshalAs(UnmanagedType.LPStruct)] Guid subgroup,
        [MarshalAs(UnmanagedType.LPStruct)] Guid setting, out IntPtr value);

    [DllImport("powrprof.dll", CharSet = CharSet.Unicode)]
    static extern uint PowerSetActiveScheme(IntPtr root,
        [MarshalAs(UnmanagedType.LPStruct)] Guid scheme);

    [DllImport("powrprof.dll", CharSet = CharSet.Unicode)]
    static extern uint PowerGetActiveScheme(IntPtr root, out IntPtr scheme);

    [DllImport("powrprof.dll")]
    static extern uint PowerGetEffectiveOverlayScheme(out Guid scheme);

    [DllImport("powrprof.dll")]
    static extern uint PowerGetActualOverlayScheme(out Guid scheme);

    [DllImport("powrprof.dll")]
    static extern uint PowerSetActiveOverlayScheme(Guid scheme);

    [DllImport("kernel32.dll")]
    static extern IntPtr LocalFree(IntPtr mem);

    static readonly Guid CpuSubgroup = new("54533251-82be-4824-96c1-47b60b740d00");
    static readonly Guid BoostSetting = new("be337238-0d82-4146-a960-4f3749d470c7");

    const string OverlayEfficiency = "961cc777-2547-4f9d-8174-7d86181b8a7a";
    const string OverlayBalanced = "00000000-0000-0000-0000-000000000000";
    const string OverlayPerformance = "ded574b5-45a0-4f42-8737-46345c09c238";
    const string PlanBalanced = "381b4222-f694-41f0-9685-ff5bb260df2e";
    const string PlanHighPerformance = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c";

    static Guid ActiveScheme()
    {
        if (PowerGetActiveScheme(IntPtr.Zero, out IntPtr ptr) != 0 || ptr == IntPtr.Zero)
            return Guid.Empty;
        try
        {
            return Marshal.PtrToStructure<Guid>(ptr);
        }
        finally
        {
            LocalFree(ptr);
        }
    }

    /// <summary>GetCpuBoost reads the processor performance boost mode, -1 on failure.</summary>
    public static int GetCpuBoost()
    {
        Guid scheme = ActiveScheme();
        if (scheme == Guid.Empty) return -1;
        if (PowerReadACValueIndex(IntPtr.Zero, scheme, CpuSubgroup, BoostSetting, out IntPtr value) != 0)
            return -1;
        return value.ToInt32();
    }

    /// <summary>SetCpuBoost writes the boost mode for both AC and DC.</summary>
    public static bool SetCpuBoost(int boost)
    {
        Guid scheme = ActiveScheme();
        if (scheme == Guid.Empty) return false;
        uint ac = PowerWriteACValueIndex(IntPtr.Zero, scheme, CpuSubgroup, BoostSetting, boost);
        uint dc = PowerWriteDCValueIndex(IntPtr.Zero, scheme, CpuSubgroup, BoostSetting, boost);
        PowerSetActiveScheme(IntPtr.Zero, scheme);
        return ac == 0 && dc == 0;
    }

    /// <summary>GetPowerMode reads the power-mode slider: 0 efficiency, 1 balanced,
    /// 2 performance, 3 the high-performance plan, -1 on failure.</summary>
    public static int GetPowerMode()
    {
        if (ActiveScheme().ToString() == PlanHighPerformance) return 3;
        string id = OverlayScheme().ToString();
        if (id == OverlayEfficiency) return 0;
        if (id == OverlayPerformance) return 2;
        return 1;
    }

    /// <summary>OverlayScheme reads the slider. Windows reports the balanced
    /// overlay from PowerGetEffectiveOverlayScheme whenever the active plan
    /// pins its own settings, so the actual overlay is read first.</summary>
    static Guid OverlayScheme()
    {
        if (PowerGetActualOverlayScheme(out Guid actual) == 0 && actual != Guid.Empty)
            return actual;
        return PowerGetEffectiveOverlayScheme(out Guid effective) == 0 ? effective : Guid.Empty;
    }

    /// <summary>SetPowerMode moves the power-mode slider or the high-performance plan.
    /// The slider only applies to the balanced plan, so leaving the high-performance
    /// plan is part of moving it.</summary>
    public static bool SetPowerMode(int mode)
    {
        if (mode == 3)
            return PowerSetActiveScheme(IntPtr.Zero, new Guid(PlanHighPerformance)) == 0;
        if (ActiveScheme().ToString() != PlanBalanced &&
            PowerSetActiveScheme(IntPtr.Zero, new Guid(PlanBalanced)) != 0)
            return false;
        Guid overlay = mode switch
        {
            0 => new Guid(OverlayEfficiency),
            2 => new Guid(OverlayPerformance),
            _ => new Guid(OverlayBalanced),
        };
        return PowerSetActiveOverlayScheme(overlay) == 0;
    }
}
