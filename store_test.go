package main

import (
	"path/filepath"
	"slices"
	"testing"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/feed"
	"stock-float-mygo-native/internal/market"
)

func TestStoreDropsEventsOfStoppedFeeds(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "config.json"), nil)
	s.generation = 2

	s.apply(1, feed.Tencent, feed.QuoteEvent(market.Quote{Symbol: "usAAPL", Price: 1, PrevClose: 1}))
	s.apply(2, feed.Tencent, feed.OfflineEvent(feed.Outage{Kind: feed.Unreachable}))

	snap := s.Snapshot()
	if len(snap.Quotes) != 0 {
		t.Errorf("a stopped feed's quote was kept: %+v", snap.Quotes)
	}
	if len(snap.Outages) != 1 {
		t.Errorf("outages = %+v", snap.Outages)
	}

	s.apply(2, feed.Tencent, feed.OnlineEvent())
	if len(s.Snapshot().Outages) != 0 {
		t.Error("the outage outlived the source coming back")
	}
}

func TestStoreUpdateSavesTheSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s := NewStore(path, nil)

	before, after := s.Update(func(c *config.Config) { c.UpColor = config.UpRed })

	if before.UpColor != config.UpGreen || after.UpColor != config.UpRed {
		t.Errorf("before %q, after %q", before.UpColor, after.UpColor)
	}
	if got := config.Load(path); got.UpColor != config.UpRed || !slices.Equal(got.Symbols, after.Symbols) {
		t.Errorf("saved %+v", got)
	}
	// A change that leaves the feeds alone does not restart them.
	if s.cancel != nil {
		t.Error("changing colors started the feeds")
	}
}

func TestStoreSetsOffEachAlertOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s := NewStore(path, nil)
	s.Update(func(c *config.Config) {
		c.Alerts = []config.Alert{
			{Symbol: "usAAPL", Price: 200},
			{Symbol: "usAAPL", Price: 150, Below: true},
			{Symbol: "usNVDA", Price: 200},
		}
	})
	var got []config.Alert
	s.OnAlert(func(_ market.Quote, hits []config.Alert) { got = append(got, hits...) })

	s.apply(s.generation, feed.Tencent, feed.QuoteEvent(market.Quote{Symbol: "usAAPL", Price: 190, PrevClose: 180}))
	if len(got) != 0 {
		t.Fatalf("set off below the price: %+v", got)
	}
	// An extended-hours price counts: it is the newest.
	s.apply(s.generation, feed.Longbridge, feed.QuoteEvent(market.Quote{Symbol: "usAAPL", Price: 190, PrevClose: 180,
		Extended: &market.Extended{Session: market.Post, Price: 201, PrevClose: 190}}))
	s.apply(s.generation, feed.Longbridge, feed.QuoteEvent(market.Quote{Symbol: "usAAPL", Price: 205, PrevClose: 180}))

	if want := []config.Alert{{Symbol: "usAAPL", Price: 200}}; !slices.Equal(got, want) {
		t.Errorf("set off %+v, want %+v", got, want)
	}
	want := []config.Alert{{Symbol: "usAAPL", Price: 150, Below: true}, {Symbol: "usNVDA", Price: 200}}
	if left := s.Config().Alerts; !slices.Equal(left, want) {
		t.Errorf("left %+v", left)
	}
	if saved := config.Load(path).Alerts; !slices.Equal(saved, want) {
		t.Errorf("saved %+v", saved)
	}
}
