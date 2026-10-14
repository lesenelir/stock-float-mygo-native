package main

import (
	"context"
	"log"
	"maps"
	"slices"
	"sync"
	"time"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/feed"
	"stock-float-mygo-native/internal/market"
)

// Store owns the settings and the latest quotes, and runs the providers
// that feed them. It is safe for concurrent use: providers report from
// their own goroutines, and the app reads and changes it from the main
// thread and from bound methods.
type Store struct {
	path string

	mu      sync.Mutex
	cfg     config.Config
	quotes  map[string]market.Quote
	outages map[feed.Source]feed.Outage
	cancel  context.CancelFunc
	// generation tells the events of the running feeds from those of feeds
	// already stopped, which may still arrive.
	generation int

	changed func()
	pending bool
}

// NewStore reads the settings at path. changed is called, from a goroutine
// of its own, after quotes or settings changed; bursts of changes, such as
// a snapshot of every symbol, call it once.
func NewStore(path string, changed func()) *Store {
	return &Store{
		path:    path,
		cfg:     config.Load(path),
		quotes:  map[string]market.Quote{},
		outages: map[feed.Source]feed.Outage{},
		changed: changed,
	}
}

// Snapshot is the store's state at one moment.
type Snapshot struct {
	Config  config.Config
	Quotes  map[string]market.Quote
	Outages []feed.Outage
}

// Snapshot returns the current state, which shares nothing with the store.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{
		Config:  s.cfg.Clone(),
		Quotes:  maps.Clone(s.quotes),
		Outages: slices.Collect(maps.Values(s.outages)),
	}
}

// Config returns the current settings.
func (s *Store) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Clone()
}

// Start (re)starts the feeds for the configured symbols.
func (s *Store) Start() {
	s.mu.Lock()
	s.startLocked()
	s.mu.Unlock()
	s.notify()
}

func (s *Store) startLocked() {
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	generation := s.generation
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	// Dropping the old quotes keeps a stale extended-hours line from
	// outliving a change of data source.
	s.quotes = map[string]market.Quote{}
	s.outages = map[feed.Source]feed.Outage{}

	var symbols, us []market.Symbol
	for _, id := range s.cfg.Symbols {
		if sym, ok := market.ParseID(id); ok {
			if sym.Market == market.US {
				us = append(us, sym)
			} else {
				symbols = append(symbols, sym)
			}
		}
	}
	switch {
	case s.cfg.Longbridge:
		s.run(ctx, generation, feed.Longbridge, feed.LongbridgeProvider{}, us)
	case s.cfg.FinnhubKey != "":
		s.run(ctx, generation, feed.Finnhub, feed.FinnhubProvider{APIKey: s.cfg.FinnhubKey}, us)
	default:
		symbols = append(us, symbols...)
	}
	poll := time.Duration(s.cfg.PollSeconds * float64(time.Second))
	s.run(ctx, generation, feed.Tencent, feed.TencentProvider{Poll: poll}, symbols)
}

func (s *Store) run(ctx context.Context, generation int, source feed.Source, p feed.Provider, symbols []market.Symbol) {
	if len(symbols) == 0 {
		return
	}
	go p.Run(ctx, symbols, func(e feed.Event) { s.apply(generation, source, e) })
}

func (s *Store) apply(generation int, source feed.Source, e feed.Event) {
	s.mu.Lock()
	if generation != s.generation {
		s.mu.Unlock()
		return
	}
	switch {
	case e.Quote != nil:
		s.quotes[e.Quote.Symbol] = *e.Quote
	case e.Online:
		delete(s.outages, source)
	case e.Offline != nil:
		s.outages[source] = *e.Offline
	}
	s.mu.Unlock()
	s.notify()
}

// Update applies and saves a change of settings, restarting the feeds when
// it affects them. It returns the settings before and after.
func (s *Store) Update(change func(*config.Config)) (before, after config.Config) {
	s.mu.Lock()
	before = s.cfg.Clone()
	next := s.cfg.Clone()
	change(&next)
	s.cfg = next
	if err := next.Save(s.path); err != nil {
		log.Printf("saving %s: %v", s.path, err)
	}
	if before.FeedsChanged(next) {
		s.startLocked()
	}
	s.mu.Unlock()
	s.notify()
	return before, next.Clone()
}

// Stop stops the feeds.
func (s *Store) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.generation++
}

// notify calls changed once for the changes of the next few milliseconds:
// Finnhub can report many trades a second, and the page needs to draw only
// the latest.
func (s *Store) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending || s.changed == nil {
		return
	}
	s.pending = true
	time.AfterFunc(50*time.Millisecond, func() {
		s.mu.Lock()
		s.pending = false
		s.mu.Unlock()
		s.changed()
	})
}
