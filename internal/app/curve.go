// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// curveEditor draws a fan curve of eight points the user can drag, with
// the pointer or with the arrow keys once a point has the focus.
//
// The eight points keep their order: point i's temperature stays between
// its neighbours', and its speed stays between 0 and 100. Dragging past a
// neighbour moves the neighbour along, as rog-control-center's graph
// does not, which keeps the curve the firmware accepts.
func (a *App) curveEditor(c *ui.Context, key string, pts *[8][2]float64) {
	// The caller's pts is a copy of the frame's draft. The editor writes
	// each edit back to the app's draft, so the next frame, and Apply,
	// see it; without this a drag would vanish between frames.
	update := func() { a.fanDraft[key] = *pts }
	t := c.Theme()
	// drag holds which point the pointer holds, -1 for none. It lives in
	// the app so a frame between two pointer events keeps it.
	drag := -1
	if d, ok := a.fanDrag[key]; ok {
		drag = d
	}
	sel := a.fanPoint[key]
	ui.Row(c).Gap(8).Margin(0, 0, 8, 0).Children(func() {
		caption(c, i18n.T(c, "fans.speed")+" (%)")
		ui.Spacer(c)
		caption(c, i18n.T(c, "fans.temp")+" (°C)")
	})
	box := ui.Box(c).Shrink(0).Height(250).Radius(10).Background(t.Surface).Border(1, t.Border).
		Focusable().Cursor(ui.CursorPointer)
	box.Label(i18n.T(c, "fans.graph"))
	// The box's size as the last frame laid it out. Bounds() cannot be
	// read in the pass that builds the element, and the pointer events
	// arrive in the pass that follows the draw that noted the size.
	size := ui.Local(box, "fan-curve-box", func() ui.Rect { return ui.Rect{W: 800, H: 250} })
	box.HandleInput(func(ev ui.InputEvent) bool {
		w, h := size.W, size.H
		switch ev.Kind {
		case ui.InputPointerDown:
			i := nearestPoint(*pts, ev.X, ev.Y, w, h)
			if i >= 0 {
				a.fanDrag[key] = i
				a.fanPoint[key] = i
				return true
			}
		case ui.InputPointerMove:
			if d, ok := a.fanDrag[key]; ok && d >= 0 {
				movePoint(pts, d, ev.X, ev.Y, w, h)
				update()
				return true
			}
		case ui.InputPointerUp:
			delete(a.fanDrag, key)
			return true
		case ui.InputKeyDown:
			i := a.fanPoint[key]
			switch ev.Key {
			case ui.KeyLeft:
				pts[i][0] = math.Max(minTemp(pts, i), pts[i][0]-1)
				update()
				return true
			case ui.KeyRight:
				pts[i][0] = math.Min(maxTemp(pts, i), pts[i][0]+1)
				update()
				return true
			case ui.KeyUp:
				pts[i][1] = math.Min(100, pts[i][1]+2)
				update()
				return true
			case ui.KeyDown:
				pts[i][1] = math.Max(0, pts[i][1]-2)
				update()
				return true
			}
		}
		return false
	})
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		*size = r
		drawCurve(p, r, *pts, t, drag, sel)
	})
	// The selected point's values, under the graph.
	i := a.fanPoint[key]
	if i < 0 || i >= 8 {
		i = 0
	}
	caption(c, i18n.TF(c, "fans.point", map[string]any{
		"temp":  int(pts[i][0]),
		"speed": int(pts[i][1]),
	}))
}

// graphBox maps a curve point to the graph's box: 46 DIPs of padding on
// the left for the speeds, 26 at the bottom for the temperatures.
const (
	curveLeft   = 46
	curveRight  = 16
	curveTop    = 14
	curveBottom = 28
)

// curveXY returns where a point is drawn in a box of width w and height h
// whose top-left corner is at x0, y0, in the window's coordinates.
func curveXY(x0, y0, temp, speed, w, h float64) (float64, float64) {
	gw := w - curveLeft - curveRight
	gh := h - curveTop - curveBottom
	return x0 + curveLeft + temp/100*gw, y0 + curveTop + (1-speed/100)*gh
}

// curveValue turns a box position back into temperature and speed.
func curveValue(x, y, w, h float64) (temp, speed float64) {
	gw := w - curveLeft - curveRight
	gh := h - curveTop - curveBottom
	if gw <= 0 || gh <= 0 {
		return 0, 0
	}
	temp = (x - curveLeft) / gw * 100
	speed = (1 - (y-curveTop)/gh) * 100
	return math.Max(0, math.Min(100, temp)), math.Max(0, math.Min(100, speed))
}

// nearestPoint returns the point within 22 DIPs of (x, y), or -1.
func nearestPoint(pts [8][2]float64, x, y, w, h float32) int {
	best, bestD := -1, float32(22)
	for i, p := range pts {
		px, py := curveXY(0, 0, p[0], p[1], float64(w), float64(h))
		d := float32(math.Hypot(px-float64(x), py-float64(y)))
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// minTemp and maxTemp bound a point by its neighbours.
func minTemp(pts *[8][2]float64, i int) float64 {
	if i == 0 {
		return 0
	}
	return pts[i-1][0] + 1
}

func maxTemp(pts *[8][2]float64, i int) float64 {
	if i == 7 {
		return 100
	}
	return pts[i+1][0] - 1
}

// movePoint sets point i from a box position, keeping the curve ordered.
func movePoint(pts *[8][2]float64, i int, x, y, w, h float32) {
	temp, speed := curveValue(float64(x), float64(y), float64(w), float64(h))
	temp = math.Max(minTemp(pts, i), math.Min(maxTemp(pts, i), temp))
	pts[i][0] = math.Round(temp)
	pts[i][1] = math.Round(speed)
}

// drawCurve paints the graph: the grid, the curve and its points.
func drawCurve(p *ui.Painter, r ui.Rect, pts [8][2]float64, t *ui.Theme, drag, sel int) {
	w, h := float64(r.W), float64(r.H)
	gx, gy := r.X+curveLeft, r.Y+curveTop
	gw := float64(r.W - curveLeft - curveRight)
	gh := float64(r.H - curveTop - curveBottom)
	if gw <= 0 || gh <= 0 {
		return
	}
	grid := t.Border.Mix(t.Background, 0.45)
	label := t.TextMuted
	size := t.FontSize * 0.78
	// Horizontal grid at 0, 25, 50, 75 and 100%, labelled at the left.
	for i := 0; i <= 4; i++ {
		v := float64(i) * 25
		y := float64(gy) + (1-v/100)*gh
		p.Line(float32(gx), float32(y), float32(float64(gx)+gw), float32(y), 1, grid)
		p.Text(r.X+6, float32(y)-size/2, fmt.Sprintf("%.0f", v), size, label)
	}
	// Vertical grid every 20 °C, labelled at the bottom.
	for i := 0; i <= 5; i++ {
		v := float64(i) * 20
		x := float64(gx) + v/100*gw
		p.Line(float32(x), float32(gy), float32(x), float32(float64(gy)+gh), 1, grid)
		p.Text(float32(x)-5, float32(float64(gy)+gh)+8, fmt.Sprintf("%.0f", v), size, label)
	}
	// The curve, as a smooth polyline through the points.
	points := make([][2]float32, 0, 8)
	for _, q := range pts {
		x, y := curveXY(float64(r.X), float64(r.Y), q[0], q[1], w, h)
		points = append(points, [2]float32{float32(x), float32(y)})
	}
	p.StrokePolyline(points, 2.4, t.Accent)
	// The fill under the curve, faint.
	fill := t.Accent
	fill.A = 26
	var area ui.Path
	area.MoveTo(points[0][0], float32(float64(gy)+gh))
	for _, q := range points {
		area.LineTo(q[0], q[1])
	}
	area.LineTo(points[len(points)-1][0], float32(float64(gy)+gh))
	area.Close()
	p.FillPath(&area, fill)
	// The points.
	for i, q := range points {
		rad := float32(4.2)
		col := t.Accent
		if i == drag {
			rad = 6
		} else if i == sel {
			rad = 5.2
		}
		var dot ui.Path
		dot.Circle(q[0], q[1], rad)
		p.FillPath(&dot, t.Background)
		var ring ui.Path
		ring.Circle(q[0], q[1], rad)
		p.StrokePath(&ring, 2, col)
	}
}
