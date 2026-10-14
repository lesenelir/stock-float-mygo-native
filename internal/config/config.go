// Package config reads and writes the user's settings, kept as JSON at
// ~/.config/stock-float/config.json: the same file and format as the Swift
// StockFloat, so either app picks up the other's settings.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"stock-float-mygo-native/internal/market"
)

// UpColor is how gains and losses are colored.
type UpColor string

const (
	UpGreen UpColor = "green"
	UpRed   UpColor = "red"
	// UpMono uses no red or green: the sign alone tells a gain from a loss.
	UpMono UpColor = "mono"
)

// UpColors lists the choices in menu order.
var UpColors = []UpColor{UpGreen, UpRed, UpMono}

// TextSize scales the panel's fonts and width.
type TextSize string

const (
	TextSmall  TextSize = "small"
	TextMedium TextSize = "medium"
	TextLarge  TextSize = "large"
)

// TextSizes lists the choices in menu order.
var TextSizes = []TextSize{TextSmall, TextMedium, TextLarge}

// Scale is the multiplier for fonts and the panel width.
func (s TextSize) Scale() float64 {
	switch s {
	case TextMedium:
		return 1.15
	case TextLarge:
		return 1.3
	}
	return 1
}

// Language is the interface language the user picked; LanguageSystem
// follows the system's first preferred language.
type Language string

const (
	LanguageSystem Language = "system"
	LanguageZh     Language = "zh"
	LanguageEn     Language = "en"
)

// Languages lists the choices in menu order.
var Languages = []Language{LanguageSystem, LanguageZh, LanguageEn}

// OpacityChoices are the opacities the Appearance menu offers.
var OpacityChoices = []float64{1, 0.85, 0.7, 0.5}

// Config is the user's settings.
type Config struct {
	// Symbols are market.Symbol IDs, in display order.
	Symbols []string `json:"symbols"`
	// Longbridge takes US quotes from the Longbridge CLI, which adds
	// pre-market, post-market and overnight prices.
	Longbridge bool `json:"longbridge"`
	// FinnhubKey is used for US quotes when Longbridge is off; empty falls
	// back to the Tencent feed.
	FinnhubKey   string   `json:"finnhubKey"`
	PollSeconds  float64  `json:"pollSeconds"`
	UpColor      UpColor  `json:"upColor"`
	ClickThrough bool     `json:"clickThrough"`
	Language     Language `json:"language"`
	// Hotkey hides and shows the panel from any app, written like
	// ctrl+opt+s; empty disables it.
	Hotkey   string   `json:"hotkey"`
	TextSize TextSize `json:"textSize"`
	// Opacity of the whole panel, 0.3 to 1.
	Opacity float64 `json:"opacity"`
	// DockIcon shows the app in the Dock and the Command-Tab switcher; the
	// menu bar icon is always there.
	DockIcon bool `json:"dockIcon"`
}

// Default returns the settings of a first launch.
func Default() Config {
	return Config{
		Symbols:     []string{"usAAPL", "usNVDA", "usTSLA"},
		PollSeconds: 3,
		UpColor:     UpGreen,
		Language:    LanguageSystem,
		Hotkey:      "ctrl+opt+s",
		TextSize:    TextSmall,
		Opacity:     1,
		DockIcon:    true,
	}
}

// Decode reads settings from JSON. Missing keys keep their defaults, and
// out-of-range values are brought back into range.
func Decode(data []byte) (Config, error) {
	c := Default()
	if err := json.Unmarshal(data, &c); err != nil {
		return Default(), err
	}
	d := Default()
	c.Symbols, _ = ParseSymbols(c.Symbols)
	c.PollSeconds = max(1, c.PollSeconds)
	c.Opacity = min(1, max(0.3, c.Opacity))
	if !slices.Contains(UpColors, c.UpColor) {
		c.UpColor = d.UpColor
	}
	if !slices.Contains(TextSizes, c.TextSize) {
		c.TextSize = d.TextSize
	}
	if !slices.Contains(Languages, c.Language) {
		c.Language = d.Language
	}
	return c, nil
}

// ParseSymbols normalizes user-entered symbols, dropping duplicates;
// invalid lists the entries that could not be read.
func ParseSymbols(entries []string) (ids, invalid []string) {
	ids = []string{}
	for _, entry := range entries {
		s, ok := market.Parse(entry)
		if !ok {
			invalid = append(invalid, entry)
			continue
		}
		if !slices.Contains(ids, s.ID()) {
			ids = append(ids, s.ID())
		}
	}
	return ids, invalid
}

// Path is where the settings live.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "stock-float", "config.json")
}

// Load reads the settings at path, writing the defaults on first run. A file
// that cannot be read is left untouched and the defaults are used.
func Load(path string) Config {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c := Default()
		_ = c.Save(path)
		return c
	}
	if err != nil {
		return Default()
	}
	c, _ := Decode(data)
	return c
}

// Save writes the settings to path atomically.
func (c Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// FeedsChanged reports whether going from c to next needs the quote feeds
// restarted.
func (c Config) FeedsChanged(next Config) bool {
	return !slices.Equal(c.Symbols, next.Symbols) ||
		c.Longbridge != next.Longbridge ||
		c.FinnhubKey != next.FinnhubKey ||
		c.PollSeconds != next.PollSeconds
}

// Clone returns a copy that shares no slices with c.
func (c Config) Clone() Config {
	c.Symbols = slices.Clone(c.Symbols)
	return c
}
