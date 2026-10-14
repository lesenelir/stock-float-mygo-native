package main

import (
	_ "embed"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/market"
)

// The pre-market, post-market and overnight marks: SF Symbols drawn white
// by scripts/make-icons.swift, tinted here with the color of their text.
var (
	//go:embed assets/sessions/pre.png
	prePNG []byte
	//go:embed assets/sessions/post.png
	postPNG []byte
	//go:embed assets/sessions/overnight.png
	overnightPNG []byte

	sessionIcons = map[market.Session]*ui.Bitmap{
		market.Pre:       mustBitmap(prePNG),
		market.Post:      mustBitmap(postPNG),
		market.Overnight: mustBitmap(overnightPNG),
	}
)

func mustBitmap(png []byte) *ui.Bitmap {
	b, err := ui.DecodeBitmap(png)
	if err != nil {
		panic(err)
	}
	return b
}

// flash is a row lighting up in the color of a price move. Main thread.
type flash struct {
	run   uint64 // replays the row's keyframes as it changes
	color ui.Color
}

// palette is the panel's colors, the web version's: macOS's label colors
// for text, and its system green, red and orange, brighter in the dark.
type palette struct{ text, muted, green, red, gray, orange ui.Color }

func colors(t *ui.Theme) palette {
	if t.Dark {
		return palette{ui.RGBA(255, 255, 255, 0.9), ui.RGBA(255, 255, 255, 0.55),
			ui.Hex("#30d158"), ui.Hex("#ff453a"), ui.Hex("#98989d"), ui.Hex("#ff9f0a")}
	}
	return palette{ui.RGBA(0, 0, 0, 0.85), ui.RGBA(0, 0, 0, 0.5),
		ui.Hex("#34c759"), ui.Hex("#ff3b30"), ui.Hex("#8e8e93"), ui.Hex("#ff9500")}
}

// tint is the color of a change: green or red as the user chose, muted for
// none, and plain, which keeps the text's color, in the no-color scheme.
func tint(change float64, up config.UpColor, p palette, plain ui.Color) ui.Color {
	if up == config.UpMono {
		return plain
	}
	if change == 0 {
		return p.muted
	}
	if (change > 0) == (up == config.UpGreen) {
		return p.green
	}
	return p.red
}

func priceText(price float64) string {
	if price < 1 {
		return fmt.Sprintf("%.3f", price)
	}
	return fmt.Sprintf("%.2f", price)
}

func percentText(percent float64) string { return fmt.Sprintf("%+.2f%%", percent) }

// panelView builds the floating panel: one line per symbol with its price
// and daily change, a second line outside the regular session, and what is
// wrong when a data source is down. MyGo runs it on the main thread for
// every frame; the store asks for one when a quote arrives. Its sizes are
// the web version's CSS, in DIPs.
func (a *App) panelView(c *ui.Context) {
	p := colors(c.Theme())
	// The HUD material behind the window shows where the view draws nothing.
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	state := a.state()
	scale := float32(state.Scale)
	now := c.Now()
	var nextStale time.Time

	content := ui.Column(c).Width(204 * scale).Padding(6).Gap(2)
	// On macOS the window takes the mouse itself (native_darwin.go): drag
	// anywhere to move the panel, right-click for its menu. Elsewhere the
	// view does both.
	if runtime.GOOS != "darwin" {
		content.DragWindow()
		if content.RightClicked() {
			go mygo.RunOnMain(a.showContextMenu)
		}
	}
	content.Children(func() {
		for _, row := range state.Rows {
			a.quoteRow(c, row, state, now)
			if q := row.Quote; q != nil && q.StaleAt.After(now) && (nextStale.IsZero() || q.StaleAt.Before(nextStale)) {
				nextStale = q.StaleAt
			}
		}
		if state.EmptyHint != "" {
			flatText(c, state.EmptyHint, panelFont(12*scale, false), p.muted, false).Margin(2, 4)
		}
		// Notes wrap, which flatText does not: they are text of elements,
		// smoothed, and rare.
		note := func(s string, color ui.Color) {
			ui.Text(c, s).FontSize(10*scale).FontWeight(500).LineHeight(lineHeight).TextColor(color).Padding(0, 4)
		}
		for _, outage := range state.Outages {
			note(outage, p.orange)
		}
		if state.UpdateHint != "" {
			note(state.UpdateHint, p.muted)
		}
	})

	// A price that stops updating fades without new data arriving: draw a
	// frame as the next one goes stale.
	if !nextStale.IsZero() {
		c.After(nextStale.Sub(now) + time.Second)
	}
	// The window fits around the panel, keeping its top-left corner where it
	// is. Bounds are the last frame's: the first frame has none yet.
	if b := content.Bounds(); b.W > 0 && b.H > 0 {
		a.fitPanel(int(math.Ceil(float64(b.W))), int(math.Ceil(float64(b.H))))
	} else {
		c.Invalidate()
	}
}

// quoteRow builds one symbol's row. The name sits on the first line; the
// extended-hours line hangs under the price column only.
func (a *App) quoteRow(c *ui.Context, row Row, state PanelState, now time.Time) {
	p := colors(c.Theme())
	scale := float32(state.Scale)
	main, small := panelFont(12*scale, true), panelFont(10*scale, true)
	q := row.Quote

	el := ui.Row(c.Key(row.Symbol)).AlignItems(ui.Start).Gap(8).Padding(2, 4).Radius(4)
	if q != nil && !q.StaleAt.IsZero() && now.After(q.StaleAt) {
		// A closed market, a lunch break or a lost connection.
		el.Opacity(0.45)
	}

	// Only regular-session moves flash; extended-hours ticks update quietly.
	f := a.flashes[row.Symbol]
	if q != nil {
		old, seen := a.lastPrices[row.Symbol]
		a.lastPrices[row.Symbol] = q.Price
		if seen && old != q.Price && q.Extended == nil {
			if f == nil {
				f = &flash{}
				a.flashes[row.Symbol] = f
			}
			f.run++
			f.color = tint(q.Price-old, state.UpColor, p, p.gray)
		}
	}
	if f != nil {
		lit := el.Keyframes("flash", []ui.Keyframe{
			{Value: 0},
			{At: 100 * time.Millisecond, Value: 0.3, Ease: ui.EaseOut},
			{At: 700 * time.Millisecond, Value: 0, Ease: ui.EaseOut},
		}, ui.KeyframeOptions{Run: f.run})
		el.Background(f.color.Alpha(lit))
	}

	el.Children(func() {
		// The name is what gives way when the panel is tight; prices never wrap.
		flatText(c, row.Label, panelFont(12*scale, false), p.text, false).Basis(0).Grow(1).MinWidth(0)
		ui.Column(c).AlignItems(ui.End).Gap(1).Shrink(0).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				if q == nil {
					flatText(c, "—", main, p.text, false)
					flatText(c, "—", main, p.text, true).Width(56 * scale)
					return
				}
				flatText(c, priceText(q.Price), main, p.text, false)
				flatText(c, percentText(q.ChangePercent), main, tint(q.ChangePercent, state.UpColor, p, p.text), true).Width(56 * scale)
			})
			if q == nil || q.Extended == nil {
				return
			}
			e := q.Extended
			ui.Row(c).Gap(8).Children(func() {
				// The icon hugs the price it labels: a label, not a third number.
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					sessionIcon(c, e, 10*scale, p.muted)
					flatText(c, priceText(e.Price), small, p.muted, false)
				})
				flatText(c, percentText(e.ChangePercent), small, tint(e.ChangePercent, state.UpColor, p, p.muted), true).Width(56 * scale)
			})
		})
	})
}

// sessionIcon draws the mark of an extended session in color.
func sessionIcon(c *ui.Context, e *RowExtended, size float32, color ui.Color) {
	icon := sessionIcons[e.Session]
	ui.Box(c).Size(size, size).Label(e.Label).Role(ui.RoleImage).Draw(func(p *ui.Painter, r ui.Rect) {
		p.ImageRegion(icon, ui.Rect{}, r, ui.ImageOptions{Tint: color})
	})
}
