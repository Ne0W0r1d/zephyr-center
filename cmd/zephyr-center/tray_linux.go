// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build linux

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	xdraw "golang.org/x/image/draw"
)

// The Linux tray speaks StatusNotifierItem directly on the session bus.
// This keeps the binary free of libayatana-appindicator, which the MyGo
// tray uses, while still appearing in KDE, GNOME's AppIndicator extension
// and other StatusNotifier hosts.
const (
	sniInterface   = "org.kde.StatusNotifierItem"
	sniPath        = dbus.ObjectPath("/StatusNotifierItem")
	sniWatcher     = "org.kde.StatusNotifierWatcher"
	sniWatcherPath = dbus.ObjectPath("/StatusNotifierWatcher")
	sniNoMenu      = dbus.ObjectPath("/NO_DBUSMENU")
)

// sniPixmap is one size of the tray icon in ARGB32 network byte order,
// the representation StatusNotifierItem's IconPixmap property uses.
type sniPixmap struct {
	Width  int32
	Height int32
	Data   []byte
}

// sniToolTip mirrors the (sa(iiay)ss) ToolTip property: icon name, icon
// pixmaps, title and subtitle.
type sniToolTip struct {
	IconName   string
	IconPixmap []sniPixmap
	Title      string
	Subtitle   string
}

type sniItem struct {
	tray *sniTray
}

// sniTray owns the bus name, exported item, properties and watcher
// registration.
type sniTray struct {
	conn       *dbus.Conn
	name       string
	props      *prop.Properties
	item       *sniItem
	onActivate func(x, y int)
	done       chan struct{}
	once       sync.Once
}

func newPlatformTray(icon []byte, tooltip string, cb trayCallbacks) (trayHandle, error) {
	pixmaps, err := trayPixmaps(icon)
	if err != nil {
		return nil, fmt.Errorf("tray icon: %w", err)
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}

	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		return nil, fmt.Errorf("tray name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, fmt.Errorf("tray name %s is already taken", name)
	}

	t := &sniTray{
		conn:       conn,
		name:       name,
		item:       &sniItem{},
		onActivate: cb.Activate,
		done:       make(chan struct{}),
	}
	t.item.tray = t
	t.props, err = prop.Export(conn, sniPath, prop.Map{
		sniInterface: {
			"Category":            {Value: "ApplicationStatus", Emit: prop.EmitConst},
			"Id":                  {Value: "zephyr-center", Emit: prop.EmitConst},
			"Title":               {Value: "Zephyr Center", Emit: prop.EmitConst},
			"Status":              {Value: "Active", Emit: prop.EmitTrue},
			"WindowId":            {Value: uint32(0), Emit: prop.EmitConst},
			"IconName":            {Value: "", Emit: prop.EmitConst},
			"IconPixmap":          {Value: pixmaps, Emit: prop.EmitConst},
			"OverlayIconName":     {Value: "", Emit: prop.EmitConst},
			"OverlayIconPixmap":   {Value: []sniPixmap{}, Emit: prop.EmitConst},
			"AttentionIconName":   {Value: "", Emit: prop.EmitConst},
			"AttentionIconPixmap": {Value: []sniPixmap{}, Emit: prop.EmitConst},
			"AttentionMovieName":  {Value: "", Emit: prop.EmitConst},
			"IconThemePath":       {Value: []string{}, Emit: prop.EmitConst},
			"ToolTip":             {Value: sniToolTip{Title: tooltip}, Writable: true, Emit: prop.EmitTrue},
			"ItemIsMenu":          {Value: false, Emit: prop.EmitConst},
			"Menu":                {Value: sniNoMenu, Emit: prop.EmitConst},
		},
	})
	if err != nil {
		t.Destroy()
		return nil, fmt.Errorf("tray properties: %w", err)
	}
	if err := conn.Export(t.item, sniPath, sniInterface); err != nil {
		t.Destroy()
		return nil, fmt.Errorf("tray item: %w", err)
	}
	if err := t.register(); err != nil {
		t.Destroy()
		return nil, fmt.Errorf("status notifier watcher: %w", err)
	}
	go t.watchWatcher()
	return t, nil
}

// register tells the StatusNotifierWatcher about the item. The spec lets
// the caller pass either the bus name or the object path; hosts in the
// wild accept one or the other, so both are tried.
func (t *sniTray) register() error {
	watcher := t.conn.Object(sniWatcher, sniWatcherPath)
	err := watcher.Call(sniWatcher+".RegisterStatusNotifierItem", 0, t.name).Err
	if err == nil {
		return nil
	}
	if again := watcher.Call(sniWatcher+".RegisterStatusNotifierItem", 0, sniPath).Err; again == nil {
		return nil
	}
	return err
}

// watchWatcher re-registers after the watcher restarts, as it does when
// the panel or the AppIndicator extension reloads.
func (t *sniTray) watchWatcher() {
	ch := make(chan *dbus.Signal, 4)
	t.conn.Signal(ch)
	defer t.conn.RemoveSignal(ch)
	if err := t.conn.AddMatchSignal(
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchObjectPath("/org/freedesktop/DBus"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, sniWatcher),
	); err != nil {
		return
	}
	for {
		select {
		case <-t.done:
			return
		case sig := <-ch:
			if sig == nil || sig.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(sig.Body) < 3 {
				continue
			}
			if owner, _ := sig.Body[2].(string); owner != "" {
				_ = t.register()
			}
		}
	}
}

func (t *sniTray) activate(x, y int32) {
	if t.onActivate != nil {
		t.onActivate(int(x), int(y))
	}
}

// Activate is the left-click action: open the quick panel.
func (i *sniItem) Activate(x, y int32) *dbus.Error {
	i.tray.activate(x, y)
	return nil
}

// SecondaryActivate is the middle-click action; it opens the same panel.
func (i *sniItem) SecondaryActivate(x, y int32) *dbus.Error {
	i.tray.activate(x, y)
	return nil
}

// ContextMenu is the right-click action. The panel carries the actions
// that a menu would, so hosts get the panel instead of a second surface.
func (i *sniItem) ContextMenu(x, y int32) *dbus.Error {
	i.tray.activate(x, y)
	return nil
}

// Scroll is part of the spec and has no meaning here.
func (i *sniItem) Scroll(delta int32, orientation string) *dbus.Error { return nil }

// SetToolTip updates the property a StatusNotifier host shows on hover.
func (t *sniTray) SetToolTip(tip string) {
	if t.props == nil {
		return
	}
	if err := t.props.Set(sniInterface, "ToolTip", dbus.MakeVariant(sniToolTip{Title: tip})); err != nil {
		log.Printf("zephyr-center: tray tooltip: %v", err)
	}
}

func (t *sniTray) Destroy() {
	t.once.Do(func() {
		close(t.done)
		if t.conn == nil {
			return
		}
		_ = t.conn.Export(nil, sniPath, sniInterface)
		_ = t.conn.Export(nil, sniPath, "org.freedesktop.DBus.Properties")
		_, _ = t.conn.ReleaseName(t.name)
	})
}

// trayPixmaps scales the embedded tray artwork to the sizes hosts pick
// from, then converts each image to ARGB32 network byte order.
func trayPixmaps(icon []byte) ([]sniPixmap, error) {
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		return nil, err
	}
	sizes := []int{22, 32, 48, 64}
	out := make([]sniPixmap, 0, len(sizes))
	for _, size := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, size, size))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
		data := make([]byte, size*size*4)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				r, g, b, a := dst.At(x, y).RGBA()
				i := (y*size + x) * 4
				// ARGB32, most significant byte first.
				data[i] = byte(a >> 8)
				data[i+1] = byte(r >> 8)
				data[i+2] = byte(g >> 8)
				data[i+3] = byte(b >> 8)
			}
		}
		out = append(out, sniPixmap{Width: int32(size), Height: int32(size), Data: data})
	}
	return out, nil
}
