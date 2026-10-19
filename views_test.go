package main

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/feed"
	"stock-float-mygo-native/internal/l10n"
	"stock-float-mygo-native/internal/market"
)

// testApp is an app whose store reads a config of its own and runs no feeds.
func testApp(t *testing.T, change func(*config.Config)) *App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Language = config.LanguageEn
	if change != nil {
		change(&cfg)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.store = NewStore(path, nil)
	return a
}

// The views run without a window in tests, as fast as unit tests.
func TestPanelShowsPricesAndSessions(t *testing.T) {
	a := testApp(t, func(c *config.Config) { c.Symbols = []string{"usAAPL", "hk00700"} })
	a.store.apply(a.store.generation, feed.Longbridge, feed.QuoteEvent(market.Quote{
		Symbol: "usAAPL", Name: "AAPL", Price: 333.27, PrevClose: 330.32, Time: time.Now(),
		Extended: &market.Extended{Session: market.Post, Price: 334, PrevClose: 333.27},
	}))

	tt := ui.NewTester(a.panelView, 240, 120)

	for _, text := range []string{"AAPL", "333.27", "+0.89%", "334.00", "+0.22%", "00700", "—"} {
		if !tt.HasText(text) {
			t.Errorf("no %q in %q", text, tt.Texts())
		}
	}
}

func TestPanelSaysWhatIsWrong(t *testing.T) {
	a := testApp(t, func(c *config.Config) { c.Symbols = nil })
	a.store.apply(a.store.generation, feed.Tencent, feed.OfflineEvent(feed.Outage{Kind: feed.Unreachable}))

	tt := ui.NewTester(a.panelView, 240, 120)

	for _, text := range []string{"Right-click to add symbols", "Disconnected. Prices may be stale."} {
		if !tt.HasText(text) {
			t.Errorf("no %q in %q", text, tt.Texts())
		}
	}
}

func TestPanelFollowsTheLanguage(t *testing.T) {
	a := testApp(t, func(c *config.Config) {
		c.Symbols = nil
		c.Language = config.LanguageZh
	})

	if tt := ui.NewTester(a.panelView, 240, 120); !tt.HasText("右键添加标的") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestChangesTakeTheChosenShades(t *testing.T) {
	p := colors(&ui.Theme{}, PanelState{GreenShade: "500", RedShade: config.ShadeSystem})

	if p.green != tailwindGreens["500"] || p.red != ui.Hex("#ff3b30") {
		t.Errorf("green %v, red %v", p.green, p.red)
	}
}

func TestSettingsSayWhichSymbolsAreNotRecognized(t *testing.T) {
	f := &settingsForm{app: testApp(t, nil), s: l10n.For(config.LanguageEn, ""), symbols: "AAPL\n苹果\n1234"}
	tt := ui.NewTester(f.view, 392, 480)

	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}

	if !tt.HasText("Not recognized: 苹果, 1234") {
		t.Errorf("texts %q", tt.Texts())
	}
	// Nothing was saved.
	if got := f.app.store.Config().Symbols; len(got) != 3 {
		t.Errorf("symbols saved: %q", got)
	}
}

func TestAlertsWindowSetsAnAlertOnTheChosenSymbol(t *testing.T) {
	a := testApp(t, func(c *config.Config) { c.Symbols = []string{"usAAPL", "hk00700"} })
	a.store.apply(a.store.generation, feed.Tencent, feed.QuoteEvent(market.Quote{Symbol: "usAAPL", Price: 190, PrevClose: 180}))
	f := &alertsForm{app: a, s: l10n.For(config.LanguageEn, "")}
	tt := ui.NewTester(f.view, 360, 400)

	if !tt.HasText("Last 190.00") || !tt.HasText("No alerts. Each one notifies you once, in Notification Center.") {
		t.Errorf("texts %q", tt.Texts())
	}

	// A target under the last price waits for a fall.
	f.price = "180.5"
	f.guessDirection(a.store.Snapshot())
	if err := tt.Click("Add"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	want := []config.Alert{{Symbol: "usAAPL", Price: 180.5, Below: true}}
	if got := a.store.Config().Alerts; !slices.Equal(got, want) {
		t.Errorf("alerts %+v, want %+v", got, want)
	}
	if !tt.HasText("Falls to 180.50") {
		t.Errorf("texts %q", tt.Texts())
	}

	if err := tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	if got := a.store.Config().Alerts; len(got) != 0 {
		t.Errorf("alerts left %+v", got)
	}
}

func TestAlertsWindowRefusesAPriceItCannotRead(t *testing.T) {
	f := &alertsForm{app: testApp(t, nil), s: l10n.For(config.LanguageEn, ""), price: "abc"}
	tt := ui.NewTester(f.view, 360, 400)

	if err := tt.Click("Add"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !tt.HasText("Enter a price above 0.") {
		t.Errorf("texts %q", tt.Texts())
	}
	if got := f.app.store.Config().Alerts; len(got) != 0 {
		t.Errorf("alerts %+v", got)
	}
}
