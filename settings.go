package main

import (
	"math"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/l10n"
	"stock-float-mygo-native/internal/market"
)

// settingsForm is the settings window and what is being edited in it. Its
// fields are only touched on the main thread, by the view.
type settingsForm struct {
	app *App
	win *mygo.Window
	s   l10n.Strings

	symbols    string
	longbridge bool
	finnhubKey string
	// invalid says which symbols could not be read, after Save.
	invalid string
	// sized is the content size the window was last given.
	sized [2]int
}

// openSettings opens the settings window, in front of the other apps,
// starting from the saved settings. A window of native UI draws no frames
// while hidden, so it shows at once, and its view fits it to the form.
func (a *App) openSettings() {
	mygo.RunOnMain(func() {
		if a.settings != nil {
			a.settings.win.Destroy()
		}
		cfg := a.store.Config()
		symbols := make([]string, len(cfg.Symbols))
		for i, id := range cfg.Symbols {
			symbols[i] = editableText(id)
		}
		f := &settingsForm{
			app:        a,
			s:          a.strings(),
			symbols:    strings.Join(symbols, "\n"),
			longbridge: cfg.Longbridge,
			finnhubKey: cfg.FinnhubKey,
		}
		f.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             f.s.SettingsTitle(),
			Width:             392,
			Height:            420,
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
		a.settings = f
		f.win.OnClosed(func() {
			if a.settings == f {
				a.settings = nil
			}
		})
	})
}

func (f *settingsForm) view(c *ui.Context) {
	t := c.Theme()
	s := f.s
	if c.Shortcut(0, ui.KeyEscape) {
		f.close()
		return
	}
	form := ui.Column(c).Width(392).Padding(16).Gap(12)
	form.Children(func() {
		ui.Field(c, s.SymbolsLabel(), func() {
			ui.TextArea(c.Key("symbols"), &f.symbols).Height(140).Font("monospace").FontSize(13).AutoFocus()
		}).Description(s.SymbolsHint()).Error(f.invalid)

		ui.Column(c).Gap(4).Children(func() {
			ui.Checkbox(c.Key("longbridge"), &f.longbridge, s.LongbridgeToggle())
			ui.Text(c, s.LongbridgeHint()).FontSize(11).TextColor(t.TextMuted)
		})

		// Finnhub is used for US quotes only when Longbridge is off.
		ui.Field(c, s.FinnhubKey(), func() {
			ui.TextInput(c.Key("finnhub"), &f.finnhubKey).Placeholder(s.FinnhubPlaceholder()).Disabled(f.longbridge)
		})

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, s.Cancel()).OnClick(f.close)
			// Return in the list starts a new line; ⌘Return saves from anywhere.
			if ui.PrimaryButton(c, s.Save()).Clicked() || c.Shortcut(ui.Cmd, ui.KeyEnter) {
				f.save()
			}
		})
	})

	// The window fits around the form, which grows with an error. Bounds
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

// save checks and saves the settings, then closes the window, or says which
// symbols it could not read.
func (f *settingsForm) save() {
	entries := strings.FieldsFunc(f.symbols, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '，' })
	ids, invalid := config.ParseSymbols(entries)
	if len(invalid) > 0 {
		f.invalid = f.s.Unrecognized(invalid)
		return
	}
	f.app.change(func(c *config.Config) {
		c.Symbols = ids
		c.Longbridge = f.longbridge
		c.FinnhubKey = strings.TrimSpace(f.finnhubKey)
	})
	f.close()
}

func (f *settingsForm) close() {
	if f.win != nil {
		f.win.Close()
	}
}

// editableText is a symbol as the settings take it: US symbols without
// their prefix, as the hint asks for them.
func editableText(id string) string {
	if s, ok := market.ParseID(id); ok && s.Market == market.US {
		return s.Code
	}
	return id
}
