// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"path/filepath"
	"testing"

	"github.com/ELY-Open-Source/CrychicUI/sound"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
)

// TestPPDBindingsPersist checks that clearing a binding survives a
// restart rather than falling back to the default mapping.
func TestPPDBindingsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), settingsFileName)
	a := New(fake.New())
	a.settingsPath = path
	a.mu.Lock()
	a.ppdBindings = defaultPPDBindings()
	a.mu.Unlock()

	a.setPPDBinding(backend.ProfilePerformance, "")

	b := New(fake.New())
	b.settingsPath = path
	b.mu.Lock()
	b.ppdBindings = defaultPPDBindings()
	b.mu.Unlock()
	b.loadSettings()
	if got := b.ppdBindingFor(backend.ProfilePerformance); got != "" {
		t.Errorf("performance binding is %q after reload, want unbound", got)
	}
}

// TestSoundMutePersists checks that both muted and unmuted states survive
// a restart; the latter needs the saved false to be written explicitly.
func TestSoundMutePersists(t *testing.T) {
	sound.SetEnabled(false)
	t.Cleanup(func() { sound.SetEnabled(false) })
	path := filepath.Join(t.TempDir(), settingsFileName)

	a := New(fake.New())
	a.settingsPath = path
	a.setSoundMuted(true)

	sound.SetEnabled(true)
	b := New(fake.New())
	b.settingsPath = path
	b.loadSettings()
	if sound.Enabled() {
		t.Fatal("the muted setting did not survive a reload")
	}

	b.setSoundMuted(false)
	sound.SetEnabled(false)
	c := New(fake.New())
	c.settingsPath = path
	c.loadSettings()
	if !sound.Enabled() {
		t.Fatal("the unmuted setting did not survive a reload")
	}
}
