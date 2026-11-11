// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Command zephyr-center is Zephyr Center rewritten with MyGo and
// CrychicUI: asusd on Linux, G-Helper on Windows, one look on both.
package main

import (
	"flag"
	"log"
	"runtime"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/app"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/asusd"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// version is the app's version, set by the build.
var version = "0.1.0"

func main() {
	demo := flag.Bool("demo", false, "run with a made-up machine instead of asusd")
	locale := flag.String("lang", "", "show this language, such as zh-Hans, and save it")
	flag.Parse()

	mygo.App.SetName("Zephyr Center")
	mygo.App.SetVersion(version)
	i18n.Register()
	if *locale != "" {
		mygo.App.SetLocale(*locale)
	}

	var be backend.Backend
	switch {
	case *demo:
		be = fake.New()
	case runtime.GOOS == "windows":
		be = newWindowsBackend()
	default:
		be = asusd.New()
	}
	a := app.New(be)
	a.Start()

	mygo.App.WhenReady(func() {
		opts := mygo.WindowOptions{
			Title:             "Zephyr Center",
			Width:             1180,
			Height:            800,
			MinWidth:          900,
			MinHeight:         600,
			TitleBarStyle:     mygo.TitleBarHidden,
			TitleBarHeight:    52,
			HideWindowButtons: true,
			Content:           ui.View(a.View),
		}
		if runtime.GOOS == "linux" {
			// KWin and other Wayland compositors that prefer SSD keep
			// their own title bar for TitleBarHidden. A frameless window
			// makes GTK decorate itself instead, so only our CSD shows.
			opts.Frameless = true
			opts.TitleBarStyle = mygo.TitleBarDefault
			opts.TitleBarHeight = 0
		}
		win := mygo.NewWindow(opts)
		a.AttachWindow(win)
		attachTray(win)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
