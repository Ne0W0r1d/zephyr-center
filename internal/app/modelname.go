// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"strings"
)

// zhModelNames names the Zephyrus generations ASUS sells in China. The
// DMI string carries the English market name; Chinese readers know the
// same machine as 幻 16 2022. Keys are model-code prefixes, so every GPU
// and CPU variant of one generation maps to the same name.
var zhModelNames = []struct{ prefix, name string }{

	// TUF Series

	// GX701
	{"GX701L", "ROG 冰刃 4 Plus"},

	// Zephyrus 14 inch
	{"GA401I", "ROG 幻 14 2020"},
	{"GA401Q", "ROG 幻 14 2021"},
	{"GA402R", "ROG 幻 14 2022"},
	{"GA402X", "ROG 幻 14 2023"},
	{"GA403U", "ROG 幻 14 Air 2024"},

	// Zephyrus 15/16 inch
	{"GX550L", "ROG 冰刃双屏"},
	{"GA502I", "ROG 冰锐 2"},
	{"GU502L", "ROG 幻 15 2020"},
	{"GU502LWS", "ROG 冰刃 4 新锐"},
	{"GX502L", "ROG 冰刃 4"},
	{"GA503Q", "ROG 幻 15 2021"},
	{"GA503R", "ROG 幻 15 2022"},
	{"GU603H", "ROG 幻 16 2021"},
	{"GU603Z", "ROG 幻 16 2022"},
	{"GU603V", "ROG 幻 16 2023 标准版"},
	{"GU604V", "ROG 幻 16 2023 星空版/经典版"},
	{"GU605M", "ROG 幻 16 Air 2024"},
	{"GA605W", "ROG 幻 16 Air 锐龙AI版 2024"},

	// Flow Series
	// Strix Series
	{"G512L", "魔霸 4"},
	// Strix Scar Series
}

// modelName says product in the window's language. A machine the table
// does not know keeps its DMI name, which is always correct even when it
// is not what the local market calls it.
func modelName(locale, product string) string {
	if !strings.HasPrefix(locale, "zh") {
		return product
	}
	code := modelCode(product)
	for _, m := range zhModelNames {
		if strings.HasPrefix(code, m.prefix) {
			return m.name
		}
	}
	return product
}

// modelCode finds the ASUS model code in a DMI string, such as GU603ZM in
// "ROG Zephyrus M16 GU603ZM". A code is two letters, two or three digits,
// and a suffix that varies with the configuration.
func modelCode(product string) string {
	for _, field := range strings.Fields(product) {
		if isModelCode(field) {
			return field
		}
	}
	return ""
}

func isModelCode(s string) bool {
	if len(s) < 4 || !isUpper(s[0]) || !isUpper(s[1]) {
		return false
	}
	i := 2
	for i < len(s) && i < 5 && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i-2 < 2 {
		return false
	}
	for ; i < len(s); i++ {
		if !isUpper(s[i]) {
			return false
		}
	}
	return true
}

func isUpper(ch byte) bool { return ch >= 'A' && ch <= 'Z' }
