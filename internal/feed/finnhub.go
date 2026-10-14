package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"

	"stock-float-mygo-native/internal/market"
)

const (
	finnhubReferenceInterval = 5 * time.Minute
	finnhubPingInterval      = 15 * time.Second
)

// FinnhubProvider streams US trades from Finnhub's WebSocket; the previous
// close comes from its REST quote endpoint.
type FinnhubProvider struct {
	APIKey string
	Client *http.Client
}

// Run streams until ctx is canceled.
func (p FinnhubProvider) Run(ctx context.Context, symbols []market.Symbol, emit func(Event)) {
	book := &FinnhubBook{}
	// The two loops emit from their own goroutines; Provider promises emit
	// is never called concurrently.
	var mu sync.Mutex
	locked := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		emit(e)
	}
	var wg sync.WaitGroup
	wg.Go(func() { p.pollReference(ctx, symbols, book, locked) })
	wg.Go(func() { p.streamTrades(ctx, symbols, book, locked) })
	wg.Wait()
}

func (p FinnhubProvider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func (p FinnhubProvider) pollReference(ctx context.Context, symbols []market.Symbol, book *FinnhubBook, emit func(Event)) {
	for {
		failed := false
		for _, s := range symbols {
			ref, err := p.fetchReference(ctx, s.Code)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				failed = true
				continue
			}
			if q, ok := book.Reference(s.Code, ref.C, ref.PC, ref.T); ok {
				emit(QuoteEvent(q))
			}
		}
		wait := finnhubReferenceInterval
		if failed {
			wait = 15 * time.Second
		}
		if !sleep(ctx, wait) {
			return
		}
	}
}

type finnhubReference struct {
	// C is the current price, PC the previous close, and T the time of C in
	// seconds since the epoch.
	C  float64 `json:"c"`
	PC float64 `json:"pc"`
	T  float64 `json:"t"`
}

func (p FinnhubProvider) fetchReference(ctx context.Context, ticker string) (finnhubReference, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	u := "https://finnhub.io/api/v1/quote?" + url.Values{"symbol": {ticker}, "token": {p.APIKey}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return finnhubReference{}, err
	}
	res, err := p.client().Do(req)
	if err != nil {
		return finnhubReference{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return finnhubReference{}, fmt.Errorf("finnhub: HTTP %d", res.StatusCode)
	}
	var ref finnhubReference
	err = json.NewDecoder(res.Body).Decode(&ref)
	return ref, err
}

func (p FinnhubProvider) streamTrades(ctx context.Context, symbols []market.Symbol, book *FinnhubBook, emit func(Event)) {
	backoff := time.Second
	for {
		established := p.connect(ctx, symbols, book, emit)
		if ctx.Err() != nil {
			return
		}
		emit(OfflineEvent(Outage{Kind: Unreachable}))
		if established {
			backoff = time.Second
		} else {
			backoff = min(backoff*2, time.Minute)
		}
		if !sleep(ctx, backoff) {
			return
		}
	}
}

// connect runs one socket until it fails, and reports whether the
// subscription was ever established.
func (p FinnhubProvider) connect(ctx context.Context, symbols []market.Symbol, book *FinnhubBook, emit func(Event)) bool {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "wss://ws.finnhub.io?"+url.Values{"token": {p.APIKey}}.Encode(),
		&websocket.DialOptions{HTTPClient: p.Client})
	if err != nil {
		return false
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	for _, s := range symbols {
		msg, _ := json.Marshal(map[string]string{"type": "subscribe", "symbol": s.Code})
		if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
			return false
		}
	}
	emit(OnlineEvent())

	// A ping that goes unanswered closes the socket, which a half-open
	// connection would otherwise hide.
	go func() {
		t := time.NewTicker(finnhubPingInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pingCtx, done := context.WithTimeout(ctx, finnhubPingInterval)
				err := conn.Ping(pingCtx)
				done()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return true
		}
		for _, trade := range LatestTrades(data) {
			if q, ok := book.Trade(trade.S, trade.P, trade.T/1000); ok {
				emit(QuoteEvent(q))
			}
		}
	}
}

// FinnhubTrade is one trade of a WebSocket frame.
type FinnhubTrade struct {
	// S is the ticker, P the price and T the time in milliseconds since
	// the epoch.
	S string  `json:"s"`
	P float64 `json:"p"`
	T float64 `json:"t"`
}

// LatestTrades returns the most recent trade per ticker in one WebSocket
// frame, sorted by ticker; other frames, such as pings and errors, give
// none.
func LatestTrades(data []byte) []FinnhubTrade {
	var msg struct {
		Type string         `json:"type"`
		Data []FinnhubTrade `json:"data"`
	}
	if json.Unmarshal(data, &msg) != nil || msg.Type != "trade" {
		return nil
	}
	latest := map[string]FinnhubTrade{}
	for _, trade := range msg.Data {
		if prev, ok := latest[trade.S]; !ok || trade.T >= prev.T {
			latest[trade.S] = trade
		}
	}
	trades := make([]FinnhubTrade, 0, len(latest))
	for _, trade := range latest {
		trades = append(trades, trade)
	}
	sort.Slice(trades, func(i, j int) bool { return trades[i].S < trades[j].S })
	return trades
}

// FinnhubBook merges REST snapshots and WebSocket trades into quotes,
// keeping whichever price is newer. It is safe for concurrent use.
type FinnhubBook struct {
	mu      sync.Mutex
	entries map[string]finnhubEntry
}

type finnhubEntry struct {
	price, priceTime, prevClose float64
}

// Reference records a REST snapshot. Times are seconds since the epoch.
func (b *FinnhubBook) Reference(ticker string, price, prevClose, at float64) (market.Quote, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[ticker]
	e.prevClose = prevClose
	if at >= e.priceTime {
		e.price, e.priceTime = price, at
	}
	b.set(ticker, e)
	return finnhubQuote(ticker, e)
}

// Trade records a trade, and returns nothing until the previous close is
// known.
func (b *FinnhubBook) Trade(ticker string, price, at float64) (market.Quote, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[ticker]
	if at < e.priceTime {
		return market.Quote{}, false
	}
	e.price, e.priceTime = price, at
	b.set(ticker, e)
	return finnhubQuote(ticker, e)
}

func (b *FinnhubBook) set(ticker string, e finnhubEntry) {
	if b.entries == nil {
		b.entries = map[string]finnhubEntry{}
	}
	b.entries[ticker] = e
}

func finnhubQuote(ticker string, e finnhubEntry) (market.Quote, bool) {
	if e.price <= 0 || e.prevClose <= 0 {
		return market.Quote{}, false
	}
	q := market.Quote{Symbol: string(market.US) + ticker, Name: ticker, Price: e.price, PrevClose: e.prevClose}
	if e.priceTime > 0 {
		q.Time = time.UnixMilli(int64(e.priceTime * 1000))
	}
	return q, true
}
