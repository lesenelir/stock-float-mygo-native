// Package feed fetches quotes from the three data sources: Tencent's public
// endpoint, Finnhub's WebSocket and the Longbridge CLI.
package feed

import (
	"context"
	"time"

	"stock-float-mygo-native/internal/market"
)

// Source names a data source.
type Source string

const (
	Tencent    Source = "tencent"
	Finnhub    Source = "finnhub"
	Longbridge Source = "longbridge"
)

// OutageKind says why a source is unreachable, left untranslated so the
// interface can word it in its own language.
type OutageKind int

const (
	Unreachable OutageKind = iota
	LongbridgeMissing
	LongbridgeLoggedOut
	// LongbridgeError is an error message from the Longbridge CLI or API,
	// shown as received in Outage.Detail.
	LongbridgeError
)

// Outage is why a source is unreachable.
type Outage struct {
	Kind   OutageKind
	Detail string
}

// Event is what a provider reports: a quote, or that its source went
// online or offline.
type Event struct {
	// Quote is set for a new price.
	Quote *market.Quote
	// Online is true when the source is receiving data again.
	Online bool
	// Offline is set when the source became unreachable.
	Offline *Outage
}

// QuoteEvent, OnlineEvent and OfflineEvent build events.
func QuoteEvent(q market.Quote) Event { return Event{Quote: &q} }
func OnlineEvent() Event              { return Event{Online: true} }
func OfflineEvent(o Outage) Event     { return Event{Offline: &o} }

// Provider streams quotes for symbols to emit until ctx is canceled. emit
// may be called from any goroutine, but never concurrently.
type Provider interface {
	Run(ctx context.Context, symbols []market.Symbol, emit func(Event))
}

// sleep waits for d, or returns false once ctx is canceled.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
