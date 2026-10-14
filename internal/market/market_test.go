package market

import (
	"testing"
	"time"
)

func TestParseNormalizesInput(t *testing.T) {
	for input, id := range map[string]string{
		"usAAPL":  "usAAPL",
		"aapl":    "usAAPL",
		" BRK.B ": "usBRK.B",
		"usb":     "usUSB",
		"hk00700": "hk00700",
		"00700":   "hk00700",
		"600519":  "sh600519",
		"000001":  "sz000001",
		"300750":  "sz300750",
	} {
		s, ok := Parse(input)
		if !ok || s.ID() != id {
			t.Errorf("Parse(%q) = %q, %v; want %q", input, s.ID(), ok, id)
		}
	}
}

func TestParseRejectsUnreadableInput(t *testing.T) {
	for _, input := range []string{"", "   ", "1234", "AAPL&x=1", "a b", "苹果"} {
		if s, ok := Parse(input); ok {
			t.Errorf("Parse(%q) = %q, want no symbol", input, s.ID())
		}
	}
}

func TestAPriceGoesStaleAfterHalfAnHour(t *testing.T) {
	traded := time.Unix(1_000_000, 0)
	q := Quote{Symbol: "usAAPL", Price: 333, PrevClose: 330, Time: traded}

	if q.IsStale(traded) || q.IsStale(traded.Add(StaleAfter)) {
		t.Error("stale within half an hour")
	}
	if !q.IsStale(traded.Add(StaleAfter + time.Second)) {
		t.Error("not stale after half an hour")
	}
}

func TestAPriceWithoutATimeIsNotCalledStale(t *testing.T) {
	q := Quote{Symbol: "usAAPL", Price: 333, PrevClose: 330}
	if q.IsStale(time.Unix(1<<40, 0)) {
		t.Error("a quote without a time is stale")
	}
}
