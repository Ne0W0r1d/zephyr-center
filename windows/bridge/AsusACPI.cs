// SPDX-License-Identifier: GPL-3.0-or-later
//
// Derived from G-Helper's app/AsusACPI.cs
// (https://github.com/seerge/g-helper, GPL-3.0-or-later), trimmed to the
// ACPI calls the Zephyr Center bridge needs: no WMI, no Aura USB, no
// configuration. The IOCTL numbers and the endpoint constants are
// G-Helper's and asusctl's.

using System.Runtime.InteropServices;

namespace ZephyrBridge;

public enum AsusFan
{
    CPU = 0,
    GPU = 1,
    Mid = 2,
}

/// <summary>AsusACPI talks to the ATKACPI device the ASUS drivers expose.</summary>
public sealed class AsusACPI
{
    const string FILE_NAME = @"\\.\ATKACPI";
    const uint CONTROL_CODE = 0x0022240C;
    const uint DSTS = 0x53545344;
    const uint DEVS = 0x53564544;

    public const uint PerformanceMode = 0x00120075;
    public const uint GPUEcoROG = 0x00090020;
    public const uint GPUMuxROG = 0x00090016;
    public const uint BatteryLimit = 0x00120057;
    public const uint ScreenOverdrive = 0x00050019;
    public const uint ScreenOverdriveSupport = 0x00050020;
    public const uint DevsCPUFanCurve = 0x00110024;
    public const uint DevsGPUFanCurve = 0x00110025;
    public const uint DevsMidFanCurve = 0x00110032;
    public const uint CPU_Fan = 0x00110013;
    public const uint GPU_Fan = 0x00110014;
    public const uint Mid_Fan = 0x00110031;
    public const int TUF_KB_BRIGHTNESS = 0x00050021;
    public const int TUF_KB = 0x00100056;
    public const int TUF_KB2 = 0x0010005A;
    public const int TUF_KB_STATE = 0x00100057;
    public const int BootSound = 0x00130022;
    public const int PPT_APUA3 = 0x001200A3;
    public const int PPT_APUA0 = 0x001200A0;
    public const int PPT_GPUC0 = 0x001200C0;
    public const int PPT_GPUC2 = 0x001200C2;
    public const int PPT_APUC1 = 0x001200C1;
    public const int PPT_CPUB0 = 0x001200B0;
    public const int PPT_TOTAL9A = 0x0012009A;
    public const int PPT_GPUCPU9C = 0x0012009C;
    public const int PPT_TEMP9E = 0x0012009E;
    public const int PPT_CROSS9F = 0x0012009F;
    public const int GPU_BASE = 0x00120099;
    public const int GPU_POWER = 0x00120098;
    public const int Temp_CPU = 0x00120094;
    public const int Temp_GPU = 0x00120097;

    public const int PerformanceBalanced = 0;
    public const int PerformanceTurbo = 1;
    public const int PerformanceSilent = 2;

    const int PresenceBit = 0x00010000;

    const uint GENERIC_READ = 0x80000000;
    const uint GENERIC_WRITE = 0x40000000;
    const uint OPEN_EXISTING = 3;
    const uint FILE_ATTRIBUTE_NORMAL = 0x80;
    const uint FILE_SHARE_READ = 1;
    const uint FILE_SHARE_WRITE = 2;

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    static extern IntPtr CreateFile(string lpFileName, uint dwDesiredAccess, uint dwShareMode,
        IntPtr lpSecurityAttributes, uint dwCreationDisposition, uint dwFlagsAndAttributes,
        IntPtr hTemplateFile);

    [DllImport("kernel32.dll", SetLastError = true)]
    static extern bool DeviceIoControl(IntPtr hDevice, uint dwIoControlCode, byte[] lpInBuffer,
        uint nInBufferSize, byte[] lpOutBuffer, uint nOutBufferSize, ref uint lpBytesReturned,
        IntPtr lpOverlapped);

    [DllImport("kernel32.dll", SetLastError = true)]
    static extern bool CloseHandle(IntPtr hObject);

    IntPtr handle = new(-1);
    readonly Dictionary<uint, bool> support = new();
    readonly bool vivo;

    public AsusACPI(string model)
    {
        vivo = model.Contains("Vivobook", StringComparison.OrdinalIgnoreCase)
            || model.Contains("Zenbook", StringComparison.OrdinalIgnoreCase)
            || model.Contains("ProArt", StringComparison.OrdinalIgnoreCase);
        try
        {
            handle = CreateFile(FILE_NAME, GENERIC_READ | GENERIC_WRITE,
                FILE_SHARE_READ | FILE_SHARE_WRITE, IntPtr.Zero, OPEN_EXISTING,
                FILE_ATTRIBUTE_NORMAL, IntPtr.Zero);
        }
        catch
        {
            handle = new IntPtr(-1);
        }
    }

    public bool Connected => handle != new IntPtr(-1);

    public uint GPUEco => vivo ? 0x00090120 : GPUEcoROG;
    public uint GPUMux => vivo ? 0x00090026 : GPUMuxROG;

    bool Control(uint code, byte[] input, byte[] output)
    {
        uint returned = 0;
        return DeviceIoControl(handle, code, input, (uint)input.Length, output,
            (uint)output.Length, ref returned, IntPtr.Zero);
    }

    byte[]? CallMethod(uint method, byte[] args)
    {
        var input = new byte[8 + args.Length];
        BitConverter.GetBytes(method).CopyTo(input, 0);
        BitConverter.GetBytes((uint)args.Length).CopyTo(input, 4);
        Array.Copy(args, 0, input, 8, args.Length);
        var output = new byte[16];
        return Control(CONTROL_CODE, input, output) ? output : null;
    }

    public int DeviceSet(uint id, int status)
    {
        var args = new byte[8];
        BitConverter.GetBytes(id).CopyTo(args, 0);
        BitConverter.GetBytes((uint)status).CopyTo(args, 4);
        var output = CallMethod(DEVS, args);
        return output is null ? -1 : BitConverter.ToInt32(output, 0);
    }

    public int DeviceSet(uint id, byte[] parameters)
    {
        var args = new byte[4 + parameters.Length];
        BitConverter.GetBytes(id).CopyTo(args, 0);
        parameters.CopyTo(args, 4);
        var output = CallMethod(DEVS, args);
        return output is null ? -1 : BitConverter.ToInt32(output, 0);
    }

    public int DeviceGet(uint id)
    {
        int raw = DeviceGetRaw(id);
        if (raw < 0) return -1;
        return (raw & PresenceBit) == 0 ? raw : raw & 0xFFFF;
    }

    /// <summary>DeviceGetRaw reads an endpoint's whole reply, or -1 when the
    /// firmware does not implement it. The high bits are flags, not part of
    /// the value: PresenceBit says the endpoint exists, and a few endpoints
    /// pack a mask of the options they offer into the low word instead of a
    /// current setting.</summary>
    public int DeviceGetRaw(uint id)
    {
        var output = DeviceReply(id);
        return output is null ? -1 : BitConverter.ToInt32(output, 0);
    }

    /// <summary>DevicePresent reports whether the firmware implements an
    /// endpoint, which is not the same as the value it holds: the mode
    /// endpoint answers with a mask, and the charge limit with a packed
    /// word, so neither can be probed by its value.</summary>
    public bool DevicePresent(uint id) => DeviceReply(id) is not null;

    byte[]? DeviceReply(uint id)
    {
        var args = new byte[8];
        BitConverter.GetBytes(id).CopyTo(args, 0);
        return CallMethod(DSTS, args);
    }

    public byte[] DeviceGetBuffer(uint id, uint status = 0)
    {
        var args = new byte[8];
        BitConverter.GetBytes(id).CopyTo(args, 0);
        BitConverter.GetBytes(status).CopyTo(args, 4);
        return CallMethod(DSTS, args) ?? new byte[16];
    }

    public bool IsSupported(uint id)
    {
        if (!support.TryGetValue(id, out bool ok))
        {
            ok = DeviceGet(id) >= 0;
            support[id] = ok;
        }
        return ok;
    }

    public int GetFan(AsusFan fan)
    {
        uint endpoint = fan switch
        {
            AsusFan.GPU => GPU_Fan,
            AsusFan.Mid => Mid_Fan,
            _ => CPU_Fan,
        };
        int raw = DeviceGet(endpoint);
        int value = raw & 0xFFFF;
        return value > 120 || (value == 0 && raw < 0) ? -1 : value;
    }

    /// <summary>GetFanCurve reads a profile's 16-byte curve: 8 temperatures, then 8 PWM values.</summary>
    public byte[] GetFanCurve(AsusFan fan, int mode)
    {
        // G-Helper swaps Silent and Turbo for the firmware.
        uint firmware = mode switch
        {
            1 => 2,
            2 => 1,
            _ => 0,
        };
        uint endpoint = fan switch
        {
            AsusFan.GPU => DevsGPUFanCurve,
            AsusFan.Mid => DevsMidFanCurve,
            _ => DevsCPUFanCurve,
        };
        return DeviceGetBuffer(endpoint, firmware);
    }

    /// <summary>SetFanCurve writes a 16-byte curve to the firmware.</summary>
    public int SetFanCurve(AsusFan fan, byte[] curve)
    {
        if (curve.Length != 16 || curve.All(b => b == 0)) return -1;
        uint endpoint = fan switch
        {
            AsusFan.GPU => DevsGPUFanCurve,
            AsusFan.Mid => DevsMidFanCurve,
            _ => DevsCPUFanCurve,
        };
        return DeviceSet(endpoint, curve);
    }

    public void Close()
    {
        if (handle != new IntPtr(-1)) CloseHandle(handle);
    }
}
