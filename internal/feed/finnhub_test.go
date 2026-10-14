package feed

import (
	"reflect"
	"testing"
	"time"

	"stock-float-mygo-native/internal/market"
)

func TestKeepsTheLatestTradePerTicker(t *testing.T) {
	frame := []byte(`{"type":"trade","data":[
		{"s":"AAPL","p":333.10,"t":1791000000500,"v":10,"c":["1"]},
		{"s":"NVDA","p":235.04,"t":1791000000100,"v":5},
		{"s":"AAPL","p":333.27,"t":1791000000900,"v":20},
		{"s":"AAPL","p":333.00,"t":1791000000200,"v":1}
	]}`)
	want := []FinnhubTrade{
		{S: "AAPL", P: 333.27, T: 1_791_000_000_900},
		{S: "NVDA", P: 235.04, T: 1_791_000_000_100},
	}
	if got := LatestTrades(frame); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestIgnoresNonTradeFrames(t *testing.T) {
	for _, frame := range []string{`{"type":"ping"}`, `{"type":"error","msg":"Invalid API key"}`, "not json"} {
		if got := LatestTrades([]byte(frame)); len(got) != 0 {
			t.Errorf("LatestTrades(%s) = %+v", frame, got)
		}
	}
}

func usQuote(price, prevClose float64, at int64) market.Quote {
	return market.Quote{Symbol: "usAAPL", Name: "AAPL", Price: price, PrevClose: prevClose, Time: time.Unix(at, 0)}
}

func TestTradesWaitForThePreviousClose(t *testing.T) {
	var book FinnhubBook

	if q, ok := book.Trade("AAPL", 333, 100); ok {
		t.Errorf("trade before the previous close gave %+v", q)
	}
	// The snapshot is older than the trade, so it supplies only the previous
	// close.
	if q, _ := book.Reference("AAPL", 331, 330, 50); !reflect.DeepEqual(q, usQuote(333, 330, 100)) {
		t.Errorf("reference gave %+v", q)
	}
	if q, _ := book.Trade("AAPL", 334, 101); !reflect.DeepEqual(q, usQuote(334, 330, 101)) {
		t.Errorf("trade gave %+v", q)
	}
}

func TestStaleTradesAndUnknownTickersAreDropped(t *testing.T) {
	var book FinnhubBook

	if q, _ := book.Reference("AAPL", 333, 330, 200); !reflect.DeepEqual(q, usQuote(333, 330, 200)) {
		t.Errorf("reference gave %+v", q)
	}
	if q, ok := book.Trade("AAPL", 320, 150); ok {
		t.Errorf("an older trade gave %+v", q)
	}
	// Finnhub answers an unknown ticker with zeros.
	if q, ok := book.Reference("NOPE", 0, 0, 0); ok {
		t.Errorf("an unknown ticker gave %+v", q)
	}
}
