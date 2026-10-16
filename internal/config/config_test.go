package config

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestParseSymbolsDeduplicatesAndReportsInvalidEntries(t *testing.T) {
	ids, invalid := ParseSymbols([]string{"aapl", "usAAPL", "hk00700", "a b"})

	if !slices.Equal(ids, []string{"usAAPL", "hk00700"}) {
		t.Errorf("ids = %q", ids)
	}
	if !slices.Equal(invalid, []string{"a b"}) {
		t.Errorf("invalid = %q", invalid)
	}
}

func TestMissingKeysFallBackToDefaults(t *testing.T) {
	c, err := Decode([]byte(`{"symbols":["nvda"],"pollSeconds":0.1}`))
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.Symbols = []string{"usNVDA"}
	want.PollSeconds = 1
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got %+v, want %+v", c, want)
	}
}

func TestReadsEveryColorScheme(t *testing.T) {
	for _, color := range UpColors {
		c, err := Decode([]byte(`{"upColor":"` + string(color) + `"}`))
		if err != nil || c.UpColor != color {
			t.Errorf("upColor %q read as %q, %v", color, c.UpColor, err)
		}
	}
}

func TestKeepsOpacityInRange(t *testing.T) {
	for stored, want := range map[string]float64{"0": 0.3, "0.7": 0.7, "5": 1} {
		c, err := Decode([]byte(`{"opacity":` + stored + `}`))
		if err != nil || c.Opacity != want {
			t.Errorf("opacity %s read as %v, %v; want %v", stored, c.Opacity, err, want)
		}
	}
}

func TestUnknownChoicesFallBackToDefaults(t *testing.T) {
	c, err := Decode([]byte(`{"upColor":"blue","textSize":"huge","language":"fr"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.UpColor != UpGreen || c.TextSize != TextSmall || c.Language != LanguageSystem {
		t.Errorf("got %+v", c)
	}
}

func TestRoundTripsThroughDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stock-float", "config.json")
	c := Default()
	c.Symbols = []string{"hk00700"}
	c.UpColor = UpRed
	c.ClickThrough = true
	c.Language = LanguageEn
	c.Hotkey = "cmd+shift+9"
	c.TextSize = TextLarge
	c.Opacity = 0.7
	c.DockIcon = false

	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); !reflect.DeepEqual(got, c) {
		t.Errorf("got %+v, want %+v", got, c)
	}
}

func TestFirstLoadWritesTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	if got := Load(path); !reflect.DeepEqual(got, Default()) {
		t.Errorf("got %+v", got)
	}
	if got := Load(path); !reflect.DeepEqual(got, Default()) {
		t.Errorf("after writing, got %+v", got)
	}
}

func TestAlertsGoOffAtTheirPriceOrBeyond(t *testing.T) {
	rise, fall := Alert{Price: 200}, Alert{Price: 200, Below: true}
	for _, tc := range []struct {
		price      float64
		rise, fall bool
	}{
		{199.99, false, true},
		{200, true, true},
		{200.01, true, false},
		{0, false, false}, // no price
	} {
		if got := rise.Reached(tc.price); got != tc.rise {
			t.Errorf("rises to 200, at %v: %v", tc.price, got)
		}
		if got := fall.Reached(tc.price); got != tc.fall {
			t.Errorf("falls to 200, at %v: %v", tc.price, got)
		}
	}
}

func TestAlertsKeepOnlyWatchedSymbolsAndPrices(t *testing.T) {
	c, err := Decode([]byte(`{"symbols":["usAAPL"],"alerts":[
		{"symbol":"usAAPL","price":200},
		{"symbol":"usAAPL","price":0},
		{"symbol":"usNVDA","price":100,"below":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []Alert{{Symbol: "usAAPL", Price: 200}}; !slices.Equal(c.Alerts, want) {
		t.Errorf("alerts = %+v", c.Alerts)
	}
}
