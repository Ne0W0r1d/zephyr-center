// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Package i18n holds the app's message catalogs and the lookup the views
// use. A key missing from a translation falls back to English, and the
// language follows MyGo's App.Locale, which the About page changes.
package i18n

import (
	"embed"
	"io/fs"
	"sort"
	"sync"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

//go:embed catalogs/*.json
var catalogFiles embed.FS

var (
	once    sync.Once
	catalog *i18n.Catalog
)

// Register reads the embedded catalogs and registers them with MyGo, so
// that App.Locale may pick any of their languages. Call it once, before
// App.Run; the view calls it too, so tests need not.
func Register() {
	once.Do(func() {
		sub, err := fs.Sub(catalogFiles, "catalogs")
		if err != nil {
			panic(err)
		}
		catalog, err = i18n.NewCatalog(sub, "en")
		if err != nil {
			panic(err)
		}
		i18n.Register(catalog)
	})
}

// T says key in the window's language. A missing key is its own name, so
// a forgotten translation is visible rather than blank.
func T(c *ui.Context, key string) string {
	Register()
	return catalog.Format(c.Locale(), key, nil)
}

// TLocale says key in locale, for callers that have a language but no
// view, such as a background job's message.
func TLocale(locale, key string) string {
	Register()
	return catalog.Format(locale, key, nil)
}

// TF says key with the ICU arguments args, as in TF(c, "overview.charge_desc",
// map[string]any{"percent": 80}).
func TF(c *ui.Context, key string, args map[string]any) string {
	Register()
	return catalog.Format(c.Locale(), key, args)
}

// Languages lists the languages the catalogs ship, English first.
func Languages() []string {
	Register()
	langs := catalog.Languages()
	out := append([]string(nil), langs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i] == "en" {
			return true
		}
		if out[j] == "en" {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

// Name is the language's name in that language, as a language picker
// shows it: "简体中文", "日本語", "Deutsch".
func Name(tag string) string {
	if tag == "" {
		return ""
	}
	if n := i18n.Autonym(tag); n != "" {
		return n
	}
	return tag
}
