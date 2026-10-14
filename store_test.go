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
