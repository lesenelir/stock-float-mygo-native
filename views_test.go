package main

import (
	"path/filepath"
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
