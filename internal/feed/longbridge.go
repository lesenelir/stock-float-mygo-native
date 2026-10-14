package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"stock-float-mygo-native/internal/market"
)

// LongbridgeSearchPaths are where the CLI is looked for: GUI apps do not
// inherit the shell's PATH, so the usual install locations are checked
// directly.
var LongbridgeSearchPaths = []string{"/opt/homebrew/bin/longbridge", "/usr/local/bin/longbridge"}

// LongbridgeProvider streams US quotes, including pre-market, post-market
// and overnight prices, from the official Longbridge CLI.
//
// `longbridge serve` keeps one market WebSocket open and speaks
// newline-delimited JSON-RPC on stdin and stdout. The CLI owns the OAuth
// token, so no Longbridge credential is stored here.
type LongbridgeProvider struct {
	// SearchPaths default to LongbridgeSearchPaths.
	SearchPaths []string
	// ReconcileInterval is the time between full-quote refreshes, which
	// correct the previous close and each session's base (default 15s).
	ReconcileInterval time.Duration
}

// Run restarts the CLI whenever it exits, until ctx is canceled.
func (p LongbridgeProvider) Run(ctx context.Context, symbols []market.Symbol, emit func(Event)) {
	backoff := time.Second
	for ctx.Err() == nil {
		if p.serve(ctx, symbols, emit) {
			backoff = time.Second
		} else {
			backoff = min(backoff*2, time.Minute)
		}
		if !sleep(ctx, backoff) {
			return
		}
	}
}

func (p LongbridgeProvider) executable() string {
	paths := p.SearchPaths
	if paths == nil {
		paths = LongbridgeSearchPaths
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path
		}
	}
	return ""
}

// serve runs one `longbridge serve` until it exits, and reports whether it
// ever delivered a snapshot.
func (p LongbridgeProvider) serve(ctx context.Context, symbols []market.Symbol, emit func(Event)) bool {
	path := p.executable()
	if path == "" {
		emit(OfflineEvent(LongbridgeOutage(false, "")))
		return false
	}
	interval := p.ReconcileInterval
	if interval == 0 {
		interval = 15 * time.Second
	}

	cmd := exec.Command(path, "serve")
	stdin, err1 := cmd.StdinPipe()
	stdout, err2 := cmd.StdoutPipe()
	stderr, err3 := cmd.StderrPipe()
	if err := errors.Join(err1, err2, err3); err != nil {
		emit(OfflineEvent(LongbridgeOutage(true, err.Error())))
		return false
	}
	if err := cmd.Start(); err != nil {
		emit(OfflineEvent(LongbridgeOutage(true, err.Error())))
		return false
	}
	terminate := func() {
		if cmd.Process.Signal(syscall.SIGTERM) != nil {
			_ = cmd.Process.Kill()
		}
	}

	remote := make([]string, len(symbols))
	for i, s := range symbols {
		remote[i] = LongbridgeSymbol(s)
	}
	var established atomic.Bool
	var lastSnapshot atomic.Int64
	lastSnapshot.Store(time.Now().UnixNano())
	var failureMu sync.Mutex
	failure := ""
	setFailure := func(s string) {
		failureMu.Lock()
		failure = s
		failureMu.Unlock()
	}

	stop := make(chan struct{})
	// The requester subscribes, then asks for full quotes now and then. A
	// CLI that stops answering is restarted rather than left showing stale
	// prices.
	go func() {
		defer terminate()
		if _, err := stdin.Write(LongbridgeRequest(1, "quote.subscribe", remote)); err != nil {
			return
		}
		for id := 2; time.Since(time.Unix(0, lastSnapshot.Load())) < 3*interval; id++ {
			if _, err := stdin.Write(LongbridgeRequest(id, "quote.quote", remote)); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()

	// Both readers run until the process exits and closes its pipes.
	var readers sync.WaitGroup
	readers.Go(func() {
		var book LongbridgeBook
		eachLine(stdout, func(line []byte) {
			switch m := ParseLongbridge(line); m.Kind {
			case LongbridgeSnapshot:
				established.Store(true)
				lastSnapshot.Store(time.Now().UnixNano())
				emit(OnlineEvent())
				for _, snapshot := range m.Quotes {
					if q, ok := book.ApplySnapshot(snapshot); ok {
						emit(QuoteEvent(q))
					}
				}
			case LongbridgeTickMessage:
				if q, ok := book.ApplyTick(m.Tick); ok {
					emit(QuoteEvent(q))
				}
			case LongbridgeFailure:
				setFailure(m.Error)
				emit(OfflineEvent(LongbridgeOutage(true, m.Error)))
			}
		})
	})
	// Draining stderr keeps the CLI from blocking on a full pipe; its last
	// line explains an early exit.
	readers.Go(func() {
		eachLine(stderr, func(line []byte) {
			if s := strings.TrimSpace(string(line)); s != "" {
				setFailure(s)
			}
		})
	})
	go func() {
		select {
		case <-ctx.Done():
			terminate()
		case <-stop:
		}
	}()

	readers.Wait()
	close(stop)
	terminate()
	_ = stdin.Close()
	_ = cmd.Wait()

	if ctx.Err() == nil {
		failureMu.Lock()
		detail := failure
		failureMu.Unlock()
		emit(OfflineEvent(LongbridgeOutage(true, detail)))
	}
	return established.Load()
}

// eachLine calls fn with every line r gives until it ends.
func eachLine(r io.Reader, fn func([]byte)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	for scanner.Scan() {
		if line := scanner.Bytes(); len(line) > 0 {
			fn(line)
		}
	}
	// A line too long to read ends the scan: drain the rest, so the CLI is
	// never blocked on a full pipe.
	_, _ = io.Copy(io.Discard, r)
}

// LongbridgeSymbol is s in Longbridge notation: AAPL.US.
func LongbridgeSymbol(s market.Symbol) string { return s.Code + ".US" }

// LongbridgeRequest is one JSON-RPC request line, newline included.
func LongbridgeRequest(id int, method string, symbols []string) []byte {
	line, _ := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Symbols []string `json:"symbols"`
		} `json:"params"`
	}{JSONRPC: "2.0", ID: id, Method: method, Params: struct {
		Symbols []string `json:"symbols"`
	}{symbols}})
	return append(line, '\n')
}

// LongbridgeOutage classifies an outage from detail, the CLI's last stderr
// line or an RPC error message.
func LongbridgeOutage(executableFound bool, detail string) Outage {
	switch {
	case !executableFound:
		return Outage{Kind: LongbridgeMissing}
	case strings.Contains(detail, "auth login"):
		return Outage{Kind: LongbridgeLoggedOut}
	case detail == "":
		return Outage{Kind: Unreachable}
	}
	return Outage{Kind: LongbridgeError, Detail: detail}
}

// Wire format

// LongbridgeKind is what a line from `longbridge serve` is, reduced to what
// the provider acts on.
type LongbridgeKind int

const (
	LongbridgeOther LongbridgeKind = iota
	// LongbridgeSnapshot carries full quotes, from quote.subscribe or
	// quote.quote.
	LongbridgeSnapshot
	// LongbridgeTickMessage is a quote.updated push.
	LongbridgeTickMessage
	LongbridgeFailure
)

// LongbridgeMessage is one line from `longbridge serve`.
type LongbridgeMessage struct {
	Kind   LongbridgeKind
	Quotes []LongbridgeQuote
	Tick   LongbridgeTick
	Error  string
}

// LongbridgePrice is a price with the time it traded.
type LongbridgePrice struct {
	Price float64
	Time  time.Time
}

// LongbridgeQuote is the OpenAPI SecurityQuote: the regular session plus
// the latest price of each extended session.
type LongbridgeQuote struct {
	Symbol    string
	Last      LongbridgePrice
	PrevClose float64
	Sessions  map[market.Session]LongbridgePrice
}

// LongbridgeTick is a quote.updated push: the latest price of whichever
// session is trading.
type LongbridgeTick struct {
	Symbol string
	Last   LongbridgePrice
	// Session is empty for the regular session.
	Session market.Session
}

// ParseLongbridge reads one line from `longbridge serve`.
func ParseLongbridge(line []byte) LongbridgeMessage {
	var envelope struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(line, &envelope) != nil {
		return LongbridgeMessage{}
	}
	if envelope.Error != nil {
		return LongbridgeMessage{Kind: LongbridgeFailure, Error: envelope.Error.Message}
	}
	if envelope.Method == "quote.updated" {
		if tick, err := decodeTick(envelope.Params); err == nil {
			return LongbridgeMessage{Kind: LongbridgeTickMessage, Tick: tick}
		}
	}
	// quote.quote answers with a bare array, quote.subscribe with an object
	// holding quotes.
	var list []wireQuote
	if json.Unmarshal(envelope.Result, &list) != nil {
		var object struct {
			Quotes *[]wireQuote `json:"quotes"`
		}
		if json.Unmarshal(envelope.Result, &object) != nil || object.Quotes == nil {
			return LongbridgeMessage{}
		}
		list = *object.Quotes
	} else if list == nil {
		return LongbridgeMessage{}
	}
	quotes := make([]LongbridgeQuote, 0, len(list))
	for _, w := range list {
		q, err := w.quote()
		if err != nil {
			return LongbridgeMessage{}
		}
		quotes = append(quotes, q)
	}
	return LongbridgeMessage{Kind: LongbridgeSnapshot, Quotes: quotes}
}

// wireNumber is a decimal the CLI serializes as a string; numbers are
// accepted too in case that changes.
type wireNumber struct {
	value float64
	set   bool
}

func (n *wireNumber) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	text := string(data)
	if unquoted, err := strconv.Unquote(text); err == nil {
		text = unquoted
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return errors.New("longbridge: not a number: " + text)
	}
	*n = wireNumber{v, true}
	return nil
}

type wirePrice struct {
	LastDone  wireNumber `json:"last_done"`
	Timestamp *string    `json:"timestamp"`
}

func (w wirePrice) price() (LongbridgePrice, error) {
	if !w.LastDone.set || w.Timestamp == nil {
		return LongbridgePrice{}, errors.New("longbridge: price without last_done or timestamp")
	}
	t, err := time.Parse(time.RFC3339Nano, *w.Timestamp)
	if err != nil {
		return LongbridgePrice{}, err
	}
	return LongbridgePrice{Price: w.LastDone.value, Time: t}, nil
}

type wireQuote struct {
	wirePrice
	Symbol          *string    `json:"symbol"`
	PrevClose       wireNumber `json:"prev_close"`
	PreMarketQuote  *wirePrice `json:"pre_market_quote"`
	PostMarketQuote *wirePrice `json:"post_market_quote"`
	OvernightQuote  *wirePrice `json:"overnight_quote"`
}

func (w wireQuote) quote() (LongbridgeQuote, error) {
	last, err := w.price()
	if err != nil {
		return LongbridgeQuote{}, err
	}
	if w.Symbol == nil || !w.PrevClose.set {
		return LongbridgeQuote{}, errors.New("longbridge: quote without symbol or prev_close")
	}
	q := LongbridgeQuote{Symbol: *w.Symbol, Last: last, PrevClose: w.PrevClose.value, Sessions: map[market.Session]LongbridgePrice{}}
	for session, wp := range map[market.Session]*wirePrice{
		market.Pre: w.PreMarketQuote, market.Post: w.PostMarketQuote, market.Overnight: w.OvernightQuote,
	} {
		if wp == nil {
			continue
		}
		p, err := wp.price()
		if err != nil {
			return LongbridgeQuote{}, err
		}
		q.Sessions[session] = p
	}
	return q, nil
}

func decodeTick(params json.RawMessage) (LongbridgeTick, error) {
	var w struct {
		wirePrice
		Symbol       *string `json:"symbol"`
		TradeSession string  `json:"trade_session"`
	}
	if err := json.Unmarshal(params, &w); err != nil {
		return LongbridgeTick{}, err
	}
	last, err := w.price()
	if err != nil {
		return LongbridgeTick{}, err
	}
	if w.Symbol == nil {
		return LongbridgeTick{}, errors.New("longbridge: push without symbol")
	}
	tick := LongbridgeTick{Symbol: *w.Symbol, Last: last}
	switch w.TradeSession {
	case "Intraday":
	case "Pre":
		tick.Session = market.Pre
	case "Post":
		tick.Session = market.Post
	case "Overnight":
		tick.Session = market.Overnight
	default:
		return LongbridgeTick{}, errors.New("longbridge: unknown trade session " + w.TradeSession)
	}
	return tick, nil
}

// Merging

// LongbridgeBook merges snapshots and pushes into quotes, keeping whichever
// price is newer per session.
type LongbridgeBook struct {
	entries map[string]*longbridgeEntry
}

type longbridgeEntry struct {
	regular   *LongbridgePrice
	prevClose float64
	sessions  map[market.Session]LongbridgePrice
}

func (b *LongbridgeBook) entry(symbol string) *longbridgeEntry {
	if b.entries == nil {
		b.entries = map[string]*longbridgeEntry{}
	}
	e := b.entries[symbol]
	if e == nil {
		e = &longbridgeEntry{sessions: map[market.Session]LongbridgePrice{}}
		b.entries[symbol] = e
	}
	return e
}

// ApplySnapshot records a full quote.
func (b *LongbridgeBook) ApplySnapshot(s LongbridgeQuote) (market.Quote, bool) {
	e := b.entry(s.Symbol)
	if e.regular == nil || !s.Last.Time.Before(e.regular.Time) {
		last := s.Last
		e.regular = &last
	}
	e.prevClose = s.PrevClose
	for session, incoming := range s.Sessions {
		if held, ok := e.sessions[session]; !ok || !incoming.Time.Before(held.Time) {
			e.sessions[session] = incoming
		}
	}
	return longbridgeQuote(s.Symbol, e)
}

// ApplyTick records a push. It returns nothing for a push older than what
// is already held, or one that arrives before the first snapshot.
func (b *LongbridgeBook) ApplyTick(t LongbridgeTick) (market.Quote, bool) {
	e := b.entry(t.Symbol)
	stored, ok := e.sessions[t.Session]
	if t.Session == "" {
		ok = e.regular != nil
		if ok {
			stored = *e.regular
		}
	}
	if ok && t.Last.Time.Before(stored.Time) {
		return market.Quote{}, false
	}
	if t.Session == "" {
		last := t.Last
		e.regular = &last
	} else {
		e.sessions[t.Session] = t.Last
	}
	return longbridgeQuote(t.Symbol, e)
}

func longbridgeQuote(remote string, e *longbridgeEntry) (market.Quote, bool) {
	ticker, ok := strings.CutSuffix(remote, ".US")
	if e.regular == nil || e.prevClose <= 0 || !ok {
		return market.Quote{}, false
	}
	regular := *e.regular
	q := market.Quote{
		Symbol:    string(market.US) + ticker,
		Name:      ticker,
		Price:     regular.Price,
		PrevClose: e.prevClose,
		Time:      regular.Time,
	}
	// The extended session shown is the most recent one, and only while it
	// is newer than the regular price. Its change is measured against the
	// regular price it follows, which is unambiguous in every session; the
	// feed's own per-session prev_close could not be told apart from the
	// day's previous close.
	var latest *LongbridgePrice
	for _, session := range market.Sessions {
		p, ok := e.sessions[session]
		if ok && p.Time.After(regular.Time) && (latest == nil || p.Time.After(latest.Time)) {
			latest = &p
			q.Extended = &market.Extended{Session: session, Price: p.Price, PrevClose: regular.Price}
			q.Time = p.Time
		}
	}
	return q, true
}
