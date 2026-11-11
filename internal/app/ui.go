// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"github.com/ELY-Open-Source/CrychicUI/choice"
	"github.com/ELY-Open-Source/CrychicUI/display"
	"github.com/ELY-Open-Source/CrychicUI/feedback"
	"github.com/ELY-Open-Source/CrychicUI/form"
	"github.com/ELY-Open-Source/CrychicUI/icon"
	"github.com/ELY-Open-Source/CrychicUI/kit"
	"github.com/ELY-Open-Source/CrychicUI/layout"
	"github.com/ELY-Open-Source/CrychicUI/typography"
	"github.com/egoist/mygo/ui"
)

// This file is the app's small vocabulary over CrychicUI: the pieces the
// pages share, so a page reads as settings rather than as layout code.

// pageHeader draws a page's title, subtitle and end actions.
func pageHeader(c *ui.Context, title, subtitle string, actions func()) {
	layout.PageHeader(c, title, layout.PageHeaderOptions{Subtitle: subtitle, Actions: actions})
}

// section groups rows under a heading and a description.
func section(c *ui.Context, title, description string, content func()) {
	form.FormSection(c, title, form.FormSectionOptions{Description: description}, content)
}

// grouped draws rows as one bordered list with separators.
func grouped(c *ui.Context, rows []func()) {
	layout.Grouped(c, rows)
}

// settingsCard is a padded card holding a page's controls.
func settingsCard(c *ui.Context, content func()) {
	layout.Card(c, content)
}

// fillCard is a settingsCard that takes the height of a stretched column,
// so cards beside each other end on the same line.
func fillCard(c *ui.Context, content func()) {
	layout.Card(c, content).Grow(1)
}

// muted is secondary text.
func muted(c *ui.Context, s string) ui.Element {
	return kit.Muted(c, s)
}

// caption is small secondary text.
func caption(c *ui.Context, s string) ui.Element {
	return typography.Caption(c, s)
}

// rowText is a title over an optional description, the left side of a
// settings row.
func rowText(c *ui.Context, title, description string) ui.Element {
	t := c.Theme()
	return ui.Column(c).Grow(1).Basis(0).Gap(t.Space(0.5)).Children(func() {
		ui.Text(c, title).FontWeight(500)
		if description != "" {
			kit.Muted(c, description)
		}
	})
}

// settingRow is a title and description with controls at its end.
func settingRow(c *ui.Context, title, description string, controls func()) ui.Element {
	t := c.Theme()
	return ui.Row(c).FillWidth().Gap(t.Space(3)).AlignItems(ui.Center).Children(func() {
		rowText(c, title, description)
		if controls != nil {
			ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Shrink(0).Children(controls)
		}
	})
}

// divider is a hairline between parts of a page.
func divider(c *ui.Context) {
	layout.Divider(c, layout.DividerOptions{})
}

// emptyState centers an icon, a title and an explanation.
func emptyState(c *ui.Context, glyph, title, text string) {
	feedback.EmptyState(c, glyph, title, text, "")
}

// alert shows a message in a tone's color.
func alert(c *ui.Context, tone feedback.Tone, title, body string) {
	feedback.Alert(c, tone, title, body)
}

// notice is a dismissible bar.
func notice(c *ui.Context, tone feedback.Tone, message string) {
	open := true
	feedback.Banner(c, message, "", &open, feedback.BannerOptions{Tone: tone})
}

// badge is a small semantic pill.
func badge(c *ui.Context, text string, variant displayVariant) {
	displayBadge(c, text, variant)
}

// displayVariant mirrors display.BadgeVariant without leaking the import
// into the pages.
type displayVariant int

// The badge variants the pages use.
const (
	badgeNeutral displayVariant = iota
	badgeSuccess
	badgeWarning
)

// displayBadge draws a badge.
func displayBadge(c *ui.Context, text string, v displayVariant) {
	variant := display.BadgeNeutral
	switch v {
	case badgeSuccess:
		variant = display.BadgeSuccess
	case badgeWarning:
		variant = display.BadgeWarning
	}
	display.Badge(c, text, display.BadgeOptions{Variant: variant})
}

// lucide returns a CrychicUI icon by name.
func lucide(name string) *ui.SVG { return icon.Must(name) }

// segments draws a segmented control and reports whether the choice
// changed.
func segments(c *ui.Context, label string, labels []string, selected *int) bool {
	if len(labels) == 0 {
		return false
	}
	if *selected < 0 || *selected >= len(labels) {
		*selected = 0
	}
	return choice.Segments(c, label, labels, selected).Changed()
}

// dropdown draws a select and reports whether the choice changed.
func dropdown(c *ui.Context, label string, options []string, selected *string) bool {
	if len(options) == 0 {
		return false
	}
	found := false
	for _, o := range options {
		if o == *selected {
			found = true
			break
		}
	}
	if !found {
		*selected = options[0]
	}
	return choice.Dropdown(c, label, options, selected).Changed()
}

// switchRow draws a switch with a description and reports a change.
func switchRow(c *ui.Context, title, description string, on *bool) bool {
	return choice.ToggleRow(c, title, description, on).Changed()
}

// optionCards draws full-width radio cards and reports whether the choice
// changed.
func optionCards(c *ui.Context, options []struct{ Value, Title, Body string }, selected *string) bool {
	changed := false
	for _, o := range options {
		c.Key(o.Value)
		changed = choice.OptionCard(c, o.Title, o.Body, o.Value, selected).Changed() || changed
	}
	return changed
}
