package main

import (
	"slices"
	"testing"
	"time"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/feed"
	"stock-float-mygo-native/internal/l10n"
	"stock-float-mygo-native/internal/market"
)

func TestPanelStateLabelsAndOrdersRows(t *testing.T) {
	cfg := config.Default()
	cfg.Symbols = []string{"hk00700", "usAAPL", "sh600519"}
	traded := time.Unix(1_790_959_760, 0)
	snap := Snapshot{
		Config: cfg,
		Quotes: map[string]market.Quote{
			"usAAPL":  {Symbol: "usAAPL", Name: "苹果", Price: 333.27, PrevClose: 330.32, Time: traded},
			"hk00700": {Symbol: "hk00700", Name: "腾讯控股", Price: 421.2, PrevClose: 431},
		},
	}

	state := panelState(snap, l10n.For(config.LanguageEn, ""), "")

	var labels []string
	for _, r := range state.Rows {
		labels = append(labels, r.Label)
	}
	// US tickers keep the ticker; other markets show their name once known.
	if !slices.Equal(labels, []string{"腾讯控股", "AAPL", "600519"}) {
		t.Errorf("labels = %q", labels)
	}
	if q := state.Rows[1].Quote; q == nil || !q.StaleAt.Equal(traded.Add(market.StaleAfter)) {
		t.Errorf("AAPL quote = %+v", q)
	}
	if !state.Rows[0].Quote.StaleAt.IsZero() {
		t.Error("a quote without a time has a stale time")
	}
	if state.Rows[2].Quote != nil {
		t.Error("a symbol without a price has a quote")
	}
	if state.EmptyHint != "" || state.UpdateHint != "" {
		t.Errorf("hints = %q %q", state.EmptyHint, state.UpdateHint)
	}
}

func TestPanelStateSaysEachOutageOnce(t *testing.T) {
	cfg := config.Default()
	cfg.Symbols = nil
	snap := Snapshot{Config: cfg, Outages: []feed.Outage{{Kind: feed.Unreachable}, {Kind: feed.Unreachable}, {Kind: feed.LongbridgeMissing}}}

	state := panelState(snap, l10n.For(config.LanguageEn, ""), "1.2.0")

	if len(state.Outages) != 2 {
		t.Errorf("outages = %q", state.Outages)
	}
	if state.EmptyHint == "" || state.UpdateHint != "Version 1.2.0 available. Right-click to update." {
		t.Errorf("hints = %q %q", state.EmptyHint, state.UpdateHint)
	}
}
