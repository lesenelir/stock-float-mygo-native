package feed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // the markets' time zones, also where the system has none

	"golang.org/x/text/encoding/simplifiedchinese"

	"stock-float-mygo-native/internal/market"
)

// TencentProvider polls Tencent's public quote endpoint. It is unofficial;
// US quotes arrive as snapshots roughly every 20 seconds.
type TencentProvider struct {
	Poll   time.Duration
	Client *http.Client
}

// Run polls until ctx is canceled.
func (p TencentProvider) Run(ctx context.Context, symbols []market.Symbol, emit func(Event)) {
	for {
		quotes, err := p.fetch(ctx, symbols)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			emit(OfflineEvent(Outage{Kind: Unreachable}))
		} else {
			emit(OnlineEvent())
			for _, q := range quotes {
				emit(QuoteEvent(q))
			}
		}
		if !sleep(ctx, p.Poll) {
			return
		}
	}
}

func (p TencentProvider) fetch(ctx context.Context, symbols []market.Symbol) ([]market.Quote, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, TencentURL(symbols), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tencent: HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	// The endpoint answers in GB18030, whatever the request asks for.
	text, err := simplifiedchinese.GB18030.NewDecoder().Bytes(body)
	if err != nil {
		return nil, err
	}
	return ParseTencent(string(text)), nil
}

// TencentURL is the request for symbols.
func TencentURL(symbols []market.Symbol) string {
	keys := make([]string, len(symbols))
	for i, s := range symbols {
		keys[i] = s.ID()
		// Plain hk codes are delayed; the r_ prefix asks for the real-time feed.
		if s.Market == market.HK {
			keys[i] = "r_" + keys[i]
		}
	}
	return "https://qt.gtimg.cn/q=" + strings.Join(keys, ",")
}

// ParseTencent reads statements like v_usAAPL="200~苹果~AAPL.OQ~333.27~…";
// whose fields are 1 name, 3 price, 4 previous close and 30 quote time.
func ParseTencent(text string) []market.Quote {
	var quotes []market.Quote
	for _, statement := range strings.Split(text, ";") {
		key, body, ok := strings.Cut(statement, "=")
		if !ok {
			continue
		}
		key, ok = strings.CutPrefix(strings.TrimSpace(key), "v_")
		if !ok {
			continue
		}
		key = strings.TrimPrefix(key, "r_")
		symbol, ok := market.ParseID(key)
		if !ok {
			continue
		}
		fields := strings.Split(strings.Trim(strings.TrimSpace(body), `"`), "~")
		if len(fields) <= 4 {
			continue
		}
		price, err1 := strconv.ParseFloat(fields[3], 64)
		prevClose, err2 := strconv.ParseFloat(fields[4], 64)
		if err1 != nil || err2 != nil || price <= 0 {
			continue
		}
		q := market.Quote{Symbol: symbol.ID(), Name: fields[1], Price: price, PrevClose: prevClose}
		if len(fields) > 30 {
			q.Time = TencentTime(fields[30], symbol.Market)
		}
		quotes = append(quotes, q)
	}
	return quotes
}

var marketZones = map[market.Market]string{
	market.US: "America/New_York",
	market.HK: "Asia/Hong_Kong",
	market.SH: "Asia/Shanghai",
	market.SZ: "Asia/Shanghai",
}

// TencentTime reads a quote time, which is local to the market and
// punctuated differently in each: 2026-10-02 13:30:40, 2026/10/02 16:08:10,
// 20260930161458. It returns zero when the text is not one.
func TencentTime(text string, m market.Market) time.Time {
	var digits strings.Builder
	for _, r := range text {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() != 14 {
		return time.Time{}
	}
	loc, err := time.LoadLocation(marketZones[m])
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("20060102150405", digits.String(), loc)
	if err != nil {
		return time.Time{}
	}
	return t
}
