package main

import (
	"slices"
	"time"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/l10n"
	"stock-float-mygo-native/internal/market"
)

// PanelState is everything the panel shows, worded in the interface
// language. The panel's view builds it from the store on every frame, so
// the view itself only lays it out.
type PanelState struct {
	Rows []Row
	// EmptyHint is shown when the watch list is empty.
	EmptyHint string
	// Outages say which data sources are unreachable, and why.
	Outages []string
	// UpdateHint says a new version is out; quiet on purpose, as the panel
	// is on screen all day.
	UpdateHint string
	UpColor    config.UpColor
	// Flash is whether price moves light up their row.
	Flash bool
	// Scale multiplies the fonts and the width of the panel.
	Scale float64
}

// Row is one symbol of the watch list.
type Row struct {
	Symbol string
	// Label is the ticker for US stocks, which are short already, and the
	// name for other markets, which read better by name.
	Label string
	// Quote is nil until the first price arrives.
	Quote *RowQuote
}

// RowQuote is the latest price of a row.
type RowQuote struct {
	Price         float64
	ChangePercent float64
	// StaleAt is when the price, if it does not change, gets old enough to
	// be misleading; zero when the source does not say when it traded.
	StaleAt  time.Time
	Extended *RowExtended
}

// RowExtended is the price of a pre-market, post-market or overnight
// session, shown on a second line.
type RowExtended struct {
	Session market.Session
	// Label names the session, for assistive technology.
	Label         string
	Price         float64
	ChangePercent float64
}

// panelState builds what the panel shows from the store's state.
func panelState(snap Snapshot, s l10n.Strings, update string) PanelState {
	state := PanelState{
		Rows:    make([]Row, 0, len(snap.Config.Symbols)),
		UpColor: snap.Config.UpColor,
		Flash:   snap.Config.Flash,
		Scale:   snap.Config.TextSize.Scale(),
	}
	for _, id := range snap.Config.Symbols {
		q, ok := snap.Quotes[id]
		row := Row{Symbol: id, Label: label(id, q, ok)}
		if ok {
			row.Quote = &RowQuote{Price: q.Price, ChangePercent: q.ChangePercent()}
			if !q.Time.IsZero() {
				row.Quote.StaleAt = q.Time.Add(market.StaleAfter)
			}
			if e := q.Extended; e != nil {
				row.Quote.Extended = &RowExtended{
					Session: e.Session, Label: s.Session(e.Session),
					Price: e.Price, ChangePercent: e.ChangePercent(),
				}
			}
		}
		state.Rows = append(state.Rows, row)
	}
	if len(snap.Config.Symbols) == 0 {
		state.EmptyHint = s.EmptyHint()
	}
	// Two sources can be down for the same reason: say it once.
	for _, o := range snap.Outages {
		if msg := s.Outage(o); !slices.Contains(state.Outages, msg) {
			state.Outages = append(state.Outages, msg)
		}
	}
	slices.Sort(state.Outages)
	if update != "" {
		state.UpdateHint = s.UpdateHint(update)
	}
	return state
}

func label(id string, q market.Quote, ok bool) string {
	sym, valid := market.ParseID(id)
	switch {
	case !valid:
		return id
	case sym.Market == market.US:
		return sym.Code
	case ok && q.Name != "":
		return q.Name
	}
	return sym.Code
}
