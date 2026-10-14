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

// The colors of gains and losses: macOS's system green and red, which are
// brighter in the dark.
type palette struct{ green, red, orange ui.Color }

func colors(t *ui.Theme) palette {
	if t.Dark {
		return palette{ui.Hex("#30d158"), ui.Hex("#ff453a"), ui.Hex("#ff9f0a")}
	}
	return palette{ui.Hex("#34c759"), ui.Hex("#ff3b30"), ui.Hex("#ff9500")}
}

// tint is the color of a change: green or red as the user chose, muted for
// none, and plain, which keeps the text's color, in the no-color scheme.
func tint(change float64, up config.UpColor, t *ui.Theme, plain ui.Color) ui.Color {
	if up == config.UpMono {
		return plain
	}
	if change == 0 {
		return t.TextMuted
	}
	p := colors(t)
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
// every frame; the store asks for one when a quote arrives.
func (a *App) panelView(c *ui.Context) {
	t := c.Theme()
	// The HUD material behind the window shows where the view draws nothing.
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	state := a.state()
	scale := float32(state.Scale)
	now := c.Now()
	var nextStale time.Time

	content := ui.Column(c).Width(204 * scale).Padding(6).Gap(2).
		FontSize(12 * scale).FontWeight(500).TextColor(t.Text)
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
			ui.Text(c, state.EmptyHint).TextColor(t.TextMuted).Padding(2, 4)
		}
		for _, outage := range state.Outages {
			ui.Text(c, outage).FontSize(10*scale).TextColor(colors(t).orange).Padding(0, 4)
		}
		if state.UpdateHint != "" {
			ui.Text(c, state.UpdateHint).FontSize(10*scale).TextColor(t.TextMuted).Padding(0, 4)
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
	t := c.Theme()
	scale := float32(state.Scale)
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
			f.color = tint(q.Price-old, state.UpColor, t, t.TextMuted)
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
		ui.Text(c, row.Label).SingleLine().Grow(1).MinWidth(0)
		ui.Column(c).AlignItems(ui.End).Gap(1).Shrink(0).Children(func() {
			ui.Row(c).Gap(8).FontFeatures("tnum").Children(func() {
				if q == nil {
					ui.Text(c, "—")
					ui.Text(c, "—").Width(56 * scale).TextAlign(ui.End)
					return
				}
				ui.Text(c, priceText(q.Price)).NoWrap()
				ui.Text(c, percentText(q.ChangePercent)).NoWrap().Width(56 * scale).TextAlign(ui.End).
					TextColor(tint(q.ChangePercent, state.UpColor, t, t.Text))
			})
			if q == nil || q.Extended == nil {
				return
			}
			e := q.Extended
			ui.Row(c).Gap(8).FontSize(10 * scale).TextColor(t.TextMuted).FontFeatures("tnum").Children(func() {
				// The icon hugs the price it labels: a label, not a third number.
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					sessionIcon(c, e, 8.5*scale, t.TextMuted)
					ui.Text(c, priceText(e.Price)).NoWrap()
				})
				ui.Text(c, percentText(e.ChangePercent)).NoWrap().Width(56 * scale).TextAlign(ui.End).
					TextColor(tint(e.ChangePercent, state.UpColor, t, t.TextMuted))
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
