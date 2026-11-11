// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package i18n

import (
	"encoding/json"
	"io/fs"
	"testing"
)

// TestCatalogsHaveTheSameKeys catches a key translated in one language and
// forgotten in another.
func TestCatalogsHaveTheSameKeys(t *testing.T) {
	Register()
	english := keysOf(t, "catalogs/en.json")
	for _, lang := range Languages() {
		if lang == "en" {
			continue
		}
		file := "catalogs/" + lang + ".json"
		other := keysOf(t, file)
		for key := range english {
			if !other[key] {
				t.Errorf("%s has no %q", file, key)
			}
		}
		for key := range other {
			if !english[key] {
				t.Errorf("%s has %q, which English does not", file, key)
			}
		}
	}
}

// keysOf reads a catalog's message keys, skipping its metadata.
func keysOf(t *testing.T, name string) map[string]bool {
	t.Helper()
	b, err := fs.ReadFile(catalogFiles, name)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := map[string]bool{}
	for key := range raw {
		if key[0] != '@' {
			out[key] = true
		}
	}
	return out
}

func TestTranslate(t *testing.T) {
	Register()
	if got := TLocale("en", "nav.overview"); got != "Overview" {
		t.Errorf("en nav.overview = %q", got)
	}
	if got := TLocale("zh-Hans", "nav.overview"); got != "概览" {
		t.Errorf("zh-Hans nav.overview = %q", got)
	}
	if got := TLocale("zh-Hans", "no.such.key"); got != "no.such.key" {
		t.Errorf("a missing key is %q, want its name", got)
	}
}
