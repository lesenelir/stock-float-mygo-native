package feed

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"stock-float-mygo-native/internal/market"
)

// Trimmed from live responses captured on 2026-10-02.
const tencentFixture = `
v_usAAPL="200~苹果~AAPL.OQ~333.27~330.32~333.26~15844548~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~~2026-10-02 12:49:20~2.95~0.89~334.54~330.61~USD";
v_r_hk00700="100~腾讯控股~00700~421.200~431.000~422.000~19108045.0~0~0~421.200~0~0~0~0~0~0~0~0~0~421.200~0~0~0~0~0~0~0~0~0~19108045.0~2026/10/02 16:08:10~-9.800~-2.27~425.000~419.800";
v_sh600519="1~贵州茅台~600519~1258.62~1235.58~1239.53~38331~21633~16698~1258.62~14~1258.44~1~1258.16~1~1258.05~2~1258.00~41~1258.65~2~1258.66~3~1258.68~1~1258.69~2~1258.75~80~~20260930161458~23.04~1.86";
v_pv_none_match="1";
v_usHALT="200~停牌~HALT.N~0.00~12.50~0.00";
`

func TestParsesEveryMarket(t *testing.T) {
	want := []market.Quote{
		// Each time is local to its market: 12:49:20 New York, 16:08:10 Hong
		// Kong, 16:14:58 Shanghai.
		{Symbol: "usAAPL", Name: "苹果", Price: 333.27, PrevClose: 330.32, Time: time.Unix(1_790_959_760, 0)},
		{Symbol: "hk00700", Name: "腾讯控股", Price: 421.2, PrevClose: 431, Time: time.Unix(1_790_928_490, 0)},
		{Symbol: "sh600519", Name: "贵州茅台", Price: 1258.62, PrevClose: 1235.58, Time: time.Unix(1_790_756_098, 0)},
	}
	got := ParseTencent(tencentFixture)
	if len(got) != len(want) {
		t.Fatalf("got %d quotes: %+v", len(got), got)
	}
	for i := range want {
		if !got[i].Time.Equal(want[i].Time) {
			t.Errorf("%s time = %v, want %v", want[i].Symbol, got[i].Time, want[i].Time)
		}
		got[i].Time, want[i].Time = time.Time{}, time.Time{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestUnreadableTimesAreLeftOut(t *testing.T) {
	if !TencentTime("", market.US).IsZero() || !TencentTime("2026-10-02", market.US).IsZero() {
		t.Error("read a time from too few digits")
	}
	if q := ParseTencent(`v_usAAPL="200~苹果~AAPL.OQ~333.27~330.32";`); len(q) != 1 || !q[0].Time.IsZero() {
		t.Errorf("got %+v", q)
	}
}

func TestChangePercentMatchesTheFeed(t *testing.T) {
	apple := ParseTencent(tencentFixture)[0]
	if got := fmt.Sprintf("%+.2f%%", apple.ChangePercent()); got != "+0.89%" {
		t.Errorf("got %s", got)
	}
}

func TestRequestsRealTimeHongKongQuotes(t *testing.T) {
	var symbols []market.Symbol
	for _, id := range []string{"usAAPL", "hk00700", "sz000001"} {
		s, _ := market.ParseID(id)
		symbols = append(symbols, s)
	}
	if got := TencentURL(symbols); got != "https://qt.gtimg.cn/q=usAAPL,r_hk00700,sz000001" {
		t.Errorf("got %s", got)
	}
}

func TestIgnoresGarbage(t *testing.T) {
	for _, text := range []string{"", "<html>502</html>", `v_usAAPL="200~苹果";`} {
		if q := ParseTencent(text); len(q) != 0 {
			t.Errorf("ParseTencent(%q) = %+v", text, q)
		}
	}
}
