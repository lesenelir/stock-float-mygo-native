package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/l10n"
	"stock-float-mygo-native/internal/market"
)

// alertsForm is the price alerts window: the alerts waiting to go off, and
// a new one being set. Its fields are only touched on the main thread, by
// the view.
type alertsForm struct {
	app *App
	win *mygo.Window
	s   l10n.Strings

	// symbol is the choice of the select, the symbol as the settings write
	// it (editableText): names arrive with the quotes, and would change the
	// options under the user.
	symbol string
	// below is the segment chosen: 0 rises to the price, 1 falls to it.
	below int
	price string
	// invalid says why the price was not taken, after Add.
	invalid string
	// symbolButton opens the menu of symbols, under itself.
	symbolButton ui.Handle
	// sized is the content size the window was last given.
	sized [2]int
}

// openAlerts opens the price alerts window in front of the other apps, as
// openSettings does the settings.
func (a *App) openAlerts() {
	mygo.RunOnMain(func() {
		if a.alerts != nil {
			a.alerts.win.Destroy()
		}
		f := &alertsForm{app: a, s: a.strings()}
		f.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             f.s.AlertsTitle(),
			Width:             360,
			Height:            240,
			UseContentSize:    true,
			Hidden:            true,
			DisableResize:     true,
			DisableMinimize:   true,
			DisableMaximize:   true,
			DisableFullScreen: true,
			Content:           ui.View(f.view),
		})
		f.win.Center()
		mygo.App.Focus()
		f.win.Show()
		a.alerts = f
		a.alertsWindow.Store(f.win)
		f.win.OnClosed(func() {
			if a.alerts == f {
				a.alerts = nil
				a.alertsWindow.Store(nil)
			}
		})
	})
}

func (f *alertsForm) view(c *ui.Context) {
	t := c.Theme()
	s := f.s
	if c.Shortcut(0, ui.KeyEscape) {
		f.close()
		return
	}
	snap := f.app.store.Snapshot()
	options := make([]string, len(snap.Config.Symbols))
	for i, id := range snap.Config.Symbols {
		options[i] = editableText(id)
	}
	if !slices.Contains(options, f.symbol) && len(options) > 0 {
		f.symbol = options[0]
	}
	id := f.symbolID(snap.Config.Symbols)
	q, quoted := snap.Quotes[id]

	form := ui.Column(c).Width(360).Padding(16).Gap(12)
	form.Children(func() {
		if len(options) == 0 {
			ui.Text(c, s.AlertsNeedSymbols()).TextColor(t.TextMuted)
		} else {
			// What the price is now, to set the alert against.
			var now []string
			if quoted && q.Name != "" && label(id, q, true) != editableText(id) {
				now = append(now, q.Name)
			}
			if quoted && q.Latest() > 0 {
				now = append(now, s.LastPrice(priceText(q.Latest())))
			}
			ui.Field(c, s.AlertSymbol(), func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					f.symbolPicker(c, snap)
					ui.Text(c, strings.Join(now, " · ")).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine()
				})
			})
			ui.Field(c, s.AlertPrice(), func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Segmented(c.Key("direction"), &f.below, s.AlertRises(), s.AlertFalls()).Label(s.AlertDirection())
					price := ui.TextInput(c.Key("price"), &f.price).Placeholder(placeholderPrice(q, quoted)).AutoFocus().Grow(1).MinWidth(0)
					if price.Changed() {
						f.guessDirection(snap)
					}
					if price.Submitted() {
						f.add(id)
					}
					ui.PrimaryButton(c, s.AddAlert()).OnClick(func() { f.add(id) })
				})
			}).Error(f.invalid)
		}

		ui.Divider(c)

		if len(snap.Config.Alerts) == 0 {
			ui.Text(c, s.NoAlerts()).FontSize(12).TextColor(t.TextMuted)
		}
		for _, alert := range snap.Config.Alerts {
			key := fmt.Sprintf("alert %s %v %v", alert.Symbol, alert.Below, alert.Price)
			ui.Row(c.Key(key)).Gap(8).AlignItems(ui.Center).Children(func() {
				aq, ok := snap.Quotes[alert.Symbol]
				ui.Text(c, label(alert.Symbol, aq, ok)).Grow(1).MinWidth(0).SingleLine()
				ui.Text(c, s.AlertCondition(alert.Below, priceText(alert.Price))).FontFeatures("tnum")
				ui.Button(c, s.RemoveAlert()).OnClick(func() { f.remove(alert) })
			})
		}

		ui.Row(c).Justify(ui.End).Children(func() {
			ui.Button(c, s.Done()).OnClick(f.close)
		})
	})

	// The window fits around the form, which grows with the alerts. Bounds
	// are the last frame's: the first frame has none yet.
	if b := form.Bounds(); f.win == nil {
		return // a test's view, without a window
	} else if b.W > 0 && b.H > 0 {
		size := [2]int{int(math.Ceil(float64(b.W))), int(math.Ceil(float64(b.H)))}
		if size != f.sized {
			f.sized = size
			go mygo.RunOnMain(func() { f.win.SetContentSize(size[0], size[1]) })
		}
	} else {
		c.Invalidate()
	}
}

// symbolPicker looks like a select, but its options are a native menu:
// the window is only as tall as the form, and would cut off a popup drawn
// in it, while a menu scrolls through a long watch list wherever it is.
func (f *alertsForm) symbolPicker(c *ui.Context, snap Snapshot) {
	t := c.Theme()
	b := ui.Button(c.Key("symbol"), "").Bind(&f.symbolButton).Label(f.s.AlertSymbol()).
		Justify(ui.SpaceBetween).MinWidth(t.Space(35))
	b.Children(func() {
		ui.Text(c, f.symbol).SingleLine()
		ui.Box(c).Size(t.Space(2.5), t.Space(2.5)).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			path.MoveTo(r.X+r.W*0.1, r.Y+r.H*0.3).LineTo(r.X+r.W*0.5, r.Y+r.H*0.7).LineTo(r.X+r.W*0.9, r.Y+r.H*0.3)
			p.StrokePath(&path, 1.5, t.TextMuted)
		})
	})
	b.OnClick(func() {
		var items []*mygo.MenuItem
		for _, id := range snap.Config.Symbols {
			text := editableText(id)
			// Names help where codes are numbers: 腾讯控股 hk00700.
			if q, ok := snap.Quotes[id]; ok && label(id, q, true) != text {
				text = label(id, q, true) + "  " + text
			}
			items = append(items, choice(text, editableText(id) == f.symbol, func() {
				f.symbol = editableText(id)
				f.guessDirection(f.app.store.Snapshot())
				f.win.Invalidate()
			}))
		}
		if f.win == nil {
			return // a test's view, without a window
		}
		menu, win, r := mygo.NewMenu(items), f.win, f.symbolButton.Bounds(c)
		// After the click is handled: the menu runs until it closes.
		go mygo.RunOnMain(func() { menu.PopupAt(win, int(r.X), int(r.Y+r.H+4)) })
	})
}

// symbolID is the ID of the symbol chosen, or "" when there is none.
func (f *alertsForm) symbolID(ids []string) string {
	for _, id := range ids {
		if editableText(id) == f.symbol {
			return id
		}
	}
	return ""
}

// guessDirection picks rises or falls by which side of the last price the
// target is, which the user can still change.
func (f *alertsForm) guessDirection(snap Snapshot) {
	target, ok := parsePrice(f.price)
	q, quoted := snap.Quotes[f.symbolID(snap.Config.Symbols)]
	if !ok || !quoted || q.Latest() <= 0 || target == q.Latest() {
		return
	}
	f.below = 0
	if target < q.Latest() {
		f.below = 1
	}
}

// add sets an alert on the symbol chosen, or says why the price was not
// taken. Alerts touch nothing App.change brings in line, so the form
// writes them to the store, which redraws the windows.
func (f *alertsForm) add(id string) {
	target, ok := parsePrice(f.price)
	if !ok {
		f.invalid = f.s.InvalidPrice()
		return
	}
	if id == "" {
		return
	}
	alert := config.Alert{Symbol: id, Price: target, Below: f.below == 1}
	f.app.store.Update(func(c *config.Config) {
		if !slices.Contains(c.Alerts, alert) {
			c.Alerts = append(c.Alerts, alert)
		}
	})
	f.price, f.invalid = "", ""
}

func (f *alertsForm) remove(alert config.Alert) {
	f.app.store.Update(func(c *config.Config) {
		if i := slices.Index(c.Alerts, alert); i >= 0 {
			c.Alerts = slices.Delete(c.Alerts, i, i+1)
		}
	})
}

func (f *alertsForm) close() {
	if f.win != nil {
		f.win.Close()
	}
}

// placeholderPrice suggests the last price, as the kind of number to type.
func placeholderPrice(q market.Quote, quoted bool) string {
	if quoted && q.Latest() > 0 {
		return priceText(q.Latest())
	}
	return ""
}

// parsePrice reads a price as typed, which must be above zero.
func parsePrice(text string) (float64, bool) {
	text = strings.ReplaceAll(strings.TrimSpace(text), ",", "")
	p, err := strconv.ParseFloat(text, 64)
	if err != nil || p <= 0 || math.IsInf(p, 0) || math.IsNaN(p) {
		return 0, false
	}
	return p, true
}

// notifyAlerts shows a system notification for each alert a quote set off.
// The store calls it from a feed's goroutine.
func (a *App) notifyAlerts(q market.Quote, hits []config.Alert) {
	s := a.strings()
	name := label(q.Symbol, q, true)
	var session string
	change := q.ChangePercent()
	if e := q.Extended; e != nil {
		session, change = s.Session(e.Session), e.ChangePercent()
	}
	body := s.AlertBody(session, priceText(q.Latest()), percentText(change))
	for _, hit := range hits {
		n := mygo.NewNotification(mygo.NotificationOptions{
			Title: s.AlertTitle(name, hit.Below, priceText(hit.Price)),
			Body:  body,
			Group: "price-alerts",
		})
		// Show waits for the system, and on the first notification for the
		// user to allow them.
		go func() {
			err := n.Show()
			switch {
			case err == nil:
			case errors.Is(err, mygo.ErrNotificationsDenied):
				// An alert the user set went unseen: say so, once a run.
				if a.notificationsDenied.CompareAndSwap(false, true) {
					mygo.Dialog.Error(s.NotificationsOff(), s.NotificationsOffDetail())
				}
			default:
				log.Printf("price alert %s: %v", n.ID(), err)
			}
		}()
	}
}
