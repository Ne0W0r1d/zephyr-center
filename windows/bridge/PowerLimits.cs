// SPDX-License-Identifier: GPL-3.0-or-later
//
// Derived from G-Helper's app/AsusACPI.cs and app/AppConfig.cs
// (https://github.com/seerge/g-helper, GPL-3.0-or-later): the per-model
// power-limit ceiling table and the support rules the Fans+Power page
// uses. Trimmed to the limits Zephyr Center exposes.

namespace ZephyrBridge;

/// <summary>PowerLimits holds G-Helper's model-adjusted ceilings.</summary>
static class PowerLimits
{
    public const int MinTotal = 5;
    public const int MinCPU = 5;
    public const int MinGPUBoost = 5;
    public const int MinGPUTemp = 75;
    public const int MaxGPUTemp = 87;
    public const int MinGPUtoCPU = 0;
    public const int StepGPUtoCPU = 5;
    public const int MaxGPUtoCPU = 10;
    public const int MinCrossLoad = 0;
    public const int MinCPUTemp = 75;
    public const int MinGPUPower = 0;

    public static int MaxTotal = 150;
    public static int DefaultTotal = 80;
    public static int MaxCPU = 100;
    public static int DefaultCPU = 80;
    public static int MaxGPUBoost = 25;
    public static int MaxCrossLoad = 40;
    public static int MaxCPUTemp = 97;
    public static int MaxGPUPower = 75;

    static bool Contains(string model, string part) =>
        model.Contains(part, StringComparison.OrdinalIgnoreCase);

    static string Short(string model)
    {
        int trim = model.LastIndexOf('_');
        return trim > 0 ? model[..trim] : model;
    }

    /// <summary>Apply sets the ceilings for a model, as G-Helper's AsusACPI constructor does.</summary>
    public static void Apply(string model)
    {
        if (Contains(model, "13QY")) MaxTotal = 250;                 // Advantage Edition
        if (Contains(model, "GA402R")) DefaultTotal = 125;           // G14 AMD
        if (Contains(model, "X13")) { MaxTotal = 75; DefaultTotal = 50; }
        if (Contains(model, "RC7")) { MaxTotal = 50; DefaultTotal = 30; } // Ally
        if (IsIntelHX(model)) { MaxTotal = 175; MaxCrossLoad = 125; MaxCPUTemp = 103; }
        if (Contains(model, "GU606")) MaxCrossLoad = 50;
        if (Contains(model, "GZ301ZE")) MaxGPUBoost = 5;
        if (DynamicBoost20(model)) MaxGPUBoost = 20;
        if (DynamicBoost15(model)) MaxGPUBoost = 15;
        if (IsCPULight(model)) MaxTotal = 90;
        if (Contains(model, "GZ302E")) MaxTotal = 93;
        if (Contains(model, "FA401EA") || Contains(model, "HN7306EA")) { MaxTotal = 115; MaxCPU = 115; }
        if (Contains(model, "H7607")) MaxTotal = 95;
    }

    static bool IsIntelHX(string m) =>
        Contains(m, "G814") || Contains(m, "G614") || Contains(m, "G834") || Contains(m, "G634") ||
        Contains(m, "G835") || Contains(m, "G635") || Contains(m, "G815") || Contains(m, "G615");

    static bool DynamicBoost20(string m) =>
        Contains(m, "GU605") || Contains(m, "GA605") || Contains(m, "GU405");

    static bool DynamicBoost15(string m) =>
        Contains(m, "FX507ZC4") || Contains(m, "GA403UM") || Contains(m, "GU605CP") ||
        Contains(m, "FX608J") || Contains(m, "FX608L") || Contains(m, "FA608U") || Contains(m, "FA608P") ||
        Contains(m, "FA401K") || Contains(m, "FA401UM") || Contains(m, "FA401UH") ||
        Contains(m, "GA403GM") || Contains(m, "GU405AM") || Contains(m, "GU405AP") ||
        Contains(m, "GU606AM") || Contains(m, "GU606AP");

    static bool IsCPULight(string m) =>
        Contains(m, "GA402X") || Contains(m, "GA605") || Contains(m, "GA403") || Contains(m, "FA507N") ||
        Short(m).Contains("FA507X") || Contains(m, "FA707N") || Contains(m, "FA707X") ||
        Contains(m, "GZ302") || Contains(m, "GU405") || Contains(m, "GX651");
}
