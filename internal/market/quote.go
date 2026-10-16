package market

import "time"

// StaleAfter is how long a price can go without an update before it is
// shown as stale.
const StaleAfter = 30 * time.Minute

// Quote is the latest price of one symbol.
type Quote struct {
	// Symbol is the Symbol.ID of the instrument.
	Symbol string
	Name   string
	// Price is the last price of the regular session.
	Price     float64
	PrevClose float64
	// Extended is set while a pre-market, post-market or overnight price is
	// newer than the regular one.
	Extended *Extended
	// Time is when the newest price shown traded, or zero when the source
	// does not say.
	Time time.Time
}

// ChangePercent is the change against the previous close, in percent.
func (q Quote) ChangePercent() float64 { return percentChange(q.Price, q.PrevClose) }

// Latest is the newest price: the extended session's while it trades,
// the regular one's otherwise.
func (q Quote) Latest() float64 {
	if q.Extended != nil {
		return q.Extended.Price
	}
	return q.Price
}

// IsStale reports whether the price is old enough to be misleading; unknown
// times are not treated as stale.
func (q Quote) IsStale(now time.Time) bool {
	return !q.Time.IsZero() && now.Sub(q.Time) > StaleAfter
}

// Session is a trading session outside the regular one.
type Session string

const (
	Pre       Session = "pre"
	Post      Session = "post"
	Overnight Session = "overnight"
)

// Sessions lists every extended session.
var Sessions = []Session{Pre, Post, Overnight}

// Extended is a price from outside the regular session.
type Extended struct {
	Session Session
	Price   float64
	// PrevClose is the close this session's change is measured against.
	PrevClose float64
}

// ChangePercent is the change against PrevClose, in percent.
func (e Extended) ChangePercent() float64 { return percentChange(e.Price, e.PrevClose) }

func percentChange(price, base float64) float64 {
	if base <= 0 {
		return 0
	}
	return (price/base - 1) * 100
}
