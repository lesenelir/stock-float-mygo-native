package feed

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"stock-float-mygo-native/internal/market"
)

func at(seconds float64) time.Time {
	return time.UnixMilli(int64(seconds * 1000))
}

func lbPrice(p, seconds float64) LongbridgePrice { return LongbridgePrice{Price: p, Time: at(seconds)} }

// Times: 100 = previous post-market, 200 = regular close, 300+ = sessions
// after that close.
func closedSnapshot(sessions map[market.Session]LongbridgePrice) LongbridgeQuote {
	if sessions == nil {
		sessions = map[market.Session]LongbridgePrice{}
	}
	return LongbridgeQuote{Symbol: "AAPL.US", Last: lbPrice(333, 200), PrevClose: 330, Sessions: sessions}
}

// equalQuotes compares quotes with their times as instants.
func equalQuotes(a, b []market.Quote) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if !x.Time.Equal(y.Time) {
			return false
		}
		x.Time, y.Time = time.Time{}, time.Time{}
		if !reflect.DeepEqual(x, y) {
			return false
		}
	}
	return true
}

func equalLB(a, b LongbridgeQuote) bool {
	if a.Symbol != b.Symbol || a.PrevClose != b.PrevClose || a.Last.Price != b.Last.Price ||
		!a.Last.Time.Equal(b.Last.Time) || len(a.Sessions) != len(b.Sessions) {
		return false
	}
	for s, p := range a.Sessions {
		q, ok := b.Sessions[s]
		if !ok || p.Price != q.Price || !p.Time.Equal(q.Time) {
			return false
		}
	}
	return true
}

// Wire format

// Captured from `longbridge serve` 0.28.7 during the regular session on
// 2026-10-02.
func TestDecodesTheSubscribeSnapshot(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":1,"result":{"subscribed":[{"symbol":"SOXL.US","fields":["quote"]}],"quotes":[{"symbol":"SOXL.US","last_done":"163.500","prev_close":"153.690","open":"165.200","high":"169.750","low":"162.400","timestamp":"2026-10-02T18:05:47Z","volume":50197187,"turnover":"8330218323.279","trade_status":"Normal","pre_market_quote":{"last_done":"165.290","timestamp":"2026-10-02T13:30:00Z","volume":4183391,"turnover":"679353549.927","high":"166.950","low":"155.370","prev_close":"153.690"},"post_market_quote":{"last_done":"154.760","timestamp":"2026-10-01T23:59:58Z","volume":765074,"turnover":"117794136.068","high":"155.300","low":"152.880","prev_close":"153.690"},"overnight_quote":{"last_done":"159.040","timestamp":"2026-10-02T08:00:00Z","volume":3081463,"turnover":"481061481.000","high":"159.610","low":"153.510","prev_close":"153.690"}}]}}`
	want := LongbridgeQuote{
		Symbol:    "SOXL.US",
		Last:      lbPrice(163.5, 1_790_964_347),
		PrevClose: 153.69,
		Sessions: map[market.Session]LongbridgePrice{
			market.Pre:       lbPrice(165.29, 1_790_947_800),
			market.Post:      lbPrice(154.76, 1_790_899_198),
			market.Overnight: lbPrice(159.04, 1_790_928_000),
		},
	}

	m := ParseLongbridge([]byte(line))
	if m.Kind != LongbridgeSnapshot || len(m.Quotes) != 1 || !equalLB(m.Quotes[0], want) {
		t.Fatalf("got %+v", m)
	}

	// Every extended price predates the regular one, so a session in
	// progress shows a single line.
	var book LongbridgeBook
	q, _ := book.ApplySnapshot(want)
	expected := market.Quote{Symbol: "usSOXL", Name: "SOXL", Price: 163.5, PrevClose: 153.69, Time: at(1_790_964_347)}
	if !equalQuotes([]market.Quote{q}, []market.Quote{expected}) {
		t.Errorf("got %+v", q)
	}
}

func TestDecodesTheQuoteArrayNumericDecimalsAndOtherTimeForms(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":2,"result":[{"symbol":"NVDA.US","last_done":235.3,"prev_close":230.86,"timestamp":"2026-10-02T20:00:00+00:00","pre_market_quote":null,"post_market_quote":{"last_done":"236.000","timestamp":"2026-10-02T23:59:58.5Z"}}]}`
	want := LongbridgeQuote{
		Symbol: "NVDA.US", Last: lbPrice(235.3, 1_790_971_200), PrevClose: 230.86,
		Sessions: map[market.Session]LongbridgePrice{market.Post: lbPrice(236, 1_790_985_598.5)},
	}

	m := ParseLongbridge([]byte(line))
	if m.Kind != LongbridgeSnapshot || len(m.Quotes) != 1 || !equalLB(m.Quotes[0], want) {
		t.Errorf("got %+v", m)
	}
}

// Captured from `longbridge serve` 0.28.7, with the session swapped in.
func TestDecodesPushes(t *testing.T) {
	for wire, session := range map[string]market.Session{"Intraday": "", "Pre": market.Pre, "Post": market.Post, "Overnight": market.Overnight} {
		line := `{"jsonrpc":"2.0","method":"quote.updated","params":{"last_done":"333.176","open":"333.210","high":"334.540","low":"330.610","timestamp":"2026-10-02T18:05:47Z","volume":18071011,"turnover":"6014100713.242","trade_status":"Normal","trade_session":"` + wire + `","current_volume":0,"current_turnover":"0","symbol":"AAPL.US"}}`
		m := ParseLongbridge([]byte(line))
		if m.Kind != LongbridgeTickMessage || m.Tick.Symbol != "AAPL.US" || m.Tick.Session != session ||
			m.Tick.Last.Price != 333.176 || !m.Tick.Last.Time.Equal(at(1_790_964_347)) {
			t.Errorf("%s: got %+v", wire, m)
		}
	}
}

func TestReportsErrorsAndSkipsEverythingElse(t *testing.T) {
	if m := ParseLongbridge([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"no quote access"}}`)); m.Kind != LongbridgeFailure || m.Error != "no quote access" {
		t.Errorf("got %+v", m)
	}
	for _, line := range []string{
		`{"jsonrpc":"2.0","id":3,"result":null}`,
		`{"jsonrpc":"2.0","id":1,"result":{"subscribed":[]}}`,
		`{"jsonrpc":"2.0","method":"quote.depth","params":{"symbol":"AAPL.US"}}`,
		"not json",
	} {
		if m := ParseLongbridge([]byte(line)); m.Kind != LongbridgeOther {
			t.Errorf("%s: got %+v", line, m)
		}
	}
}

func TestBuildsRequestLines(t *testing.T) {
	line := LongbridgeRequest(1, "quote.subscribe", []string{"AAPL.US", "BRK.B.US"})
	if !strings.HasSuffix(string(line), "\n") || strings.Count(string(line), "\n") != 1 {
		t.Errorf("not one line: %q", line)
	}
	var object struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Symbols []string `json:"symbols"`
		} `json:"params"`
	}
	if err := json.Unmarshal(line, &object); err != nil {
		t.Fatal(err)
	}
	if object.JSONRPC != "2.0" || object.ID != 1 || object.Method != "quote.subscribe" ||
		!reflect.DeepEqual(object.Params.Symbols, []string{"AAPL.US", "BRK.B.US"}) {
		t.Errorf("got %+v", object)
	}
}

func TestMapsSymbolsToLongbridgeNotation(t *testing.T) {
	for id, want := range map[string]string{"usAAPL": "AAPL.US", "usBRK.B": "BRK.B.US"} {
		s, _ := market.ParseID(id)
		if got := LongbridgeSymbol(s); got != want {
			t.Errorf("%s: got %s", id, got)
		}
	}
}

func TestClassifiesOutages(t *testing.T) {
	for _, c := range []struct {
		found  bool
		detail string
		want   Outage
	}{
		{false, "", Outage{Kind: LongbridgeMissing}},
		{true, "Error: Not authenticated. Please run `longbridge auth login` first.", Outage{Kind: LongbridgeLoggedOut}},
		{true, "no quote access", Outage{Kind: LongbridgeError, Detail: "no quote access"}},
		{true, "", Outage{Kind: Unreachable}},
	} {
		if got := LongbridgeOutage(c.found, c.detail); got != c.want {
			t.Errorf("%v %q: got %+v", c.found, c.detail, got)
		}
	}
}

// Merging

func TestShowsTheNewestExtendedSessionAgainstTheRegularPrice(t *testing.T) {
	var book LongbridgeBook

	q, _ := book.ApplySnapshot(closedSnapshot(map[market.Session]LongbridgePrice{
		market.Pre: lbPrice(332, 150), market.Post: lbPrice(334, 300), market.Overnight: lbPrice(335, 400),
	}))

	if q.Price != 333 || q.PrevClose != 330 {
		t.Errorf("got %+v", q)
	}
	if q.Extended == nil || *q.Extended != (market.Extended{Session: market.Overnight, Price: 335, PrevClose: 333}) {
		t.Errorf("extended = %+v", q.Extended)
	}
}

func TestPushesUpdateTheirOwnSession(t *testing.T) {
	var book LongbridgeBook
	book.ApplySnapshot(closedSnapshot(map[market.Session]LongbridgePrice{market.Post: lbPrice(331, 100)}))

	// The first post-market trade after the close replaces the previous
	// day's.
	post, _ := book.ApplyTick(LongbridgeTick{Symbol: "AAPL.US", Last: lbPrice(334.5, 310), Session: market.Post})
	if post.Price != 333 || post.Extended == nil || *post.Extended != (market.Extended{Session: market.Post, Price: 334.5, PrevClose: 333}) {
		t.Errorf("post = %+v %+v", post, post.Extended)
	}

	if q, ok := book.ApplyTick(LongbridgeTick{Symbol: "AAPL.US", Last: lbPrice(1, 305), Session: market.Post}); ok {
		t.Errorf("an older push gave %+v", q)
	}

	// A snapshot older than the push keeps the pushed price.
	reconciled, _ := book.ApplySnapshot(closedSnapshot(map[market.Session]LongbridgePrice{market.Post: lbPrice(334.2, 308)}))
	if reconciled.Extended == nil || reconciled.Extended.Price != 334.5 {
		t.Errorf("reconciled = %+v", reconciled.Extended)
	}

	// The next regular session opens: its price is newer than every extended
	// one, so the second line goes away.
	regular, _ := book.ApplyTick(LongbridgeTick{Symbol: "AAPL.US", Last: lbPrice(336, 500)})
	want := market.Quote{Symbol: "usAAPL", Name: "AAPL", Price: 336, PrevClose: 330, Time: at(500)}
	if !equalQuotes([]market.Quote{regular}, []market.Quote{want}) {
		t.Errorf("regular = %+v", regular)
	}
}

func TestPushesBeforeTheSnapshotAreHeldBack(t *testing.T) {
	var book LongbridgeBook

	if q, ok := book.ApplyTick(LongbridgeTick{Symbol: "AAPL.US", Last: lbPrice(334, 300), Session: market.Post}); ok {
		t.Errorf("got %+v", q)
	}
	if q, ok := book.ApplyTick(LongbridgeTick{Symbol: "AAPL.US", Last: lbPrice(333.5, 250)}); ok {
		t.Errorf("got %+v", q)
	}

	q, _ := book.ApplySnapshot(closedSnapshot(nil))
	if q.Price != 333.5 || q.PrevClose != 330 || q.Extended == nil ||
		*q.Extended != (market.Extended{Session: market.Post, Price: 334, PrevClose: 333.5}) {
		t.Errorf("got %+v %+v", q, q.Extended)
	}
}
