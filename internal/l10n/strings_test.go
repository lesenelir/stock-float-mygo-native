package l10n

import (
	"testing"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/feed"
	"stock-float-mygo-native/internal/market"
)

func TestSystemFollowsThePreferredLanguage(t *testing.T) {
	for preferred, zh := range map[string]bool{"zh-Hans-CN": true, "zh-Hant-TW": true, "en-US": false, "ja-JP": false, "": false} {
		if got := For(config.LanguageSystem, preferred).Chinese(); got != zh {
			t.Errorf("%q: Chinese() = %v", preferred, got)
		}
	}
}

func TestAnExplicitLanguageWins(t *testing.T) {
	if !For(config.LanguageZh, "en-US").Chinese() || For(config.LanguageEn, "zh-Hans").Chinese() {
		t.Error("the setting did not override the system")
	}
}

func TestOutagesSayWhatToDo(t *testing.T) {
	zh, en := For(config.LanguageZh, ""), For(config.LanguageEn, "")
	for _, c := range []struct {
		outage feed.Outage
		zh, en string
	}{
		{feed.Outage{Kind: feed.Unreachable}, "连接中断，价格可能已过期", "Disconnected. Prices may be stale."},
		{feed.Outage{Kind: feed.LongbridgeError, Detail: "no quote access"}, "长桥：no quote access", "Longbridge: no quote access"},
	} {
		if got := zh.Outage(c.outage); got != c.zh {
			t.Errorf("zh: %q", got)
		}
		if got := en.Outage(c.outage); got != c.en {
			t.Errorf("en: %q", got)
		}
	}
}

func TestEverySessionHasALabel(t *testing.T) {
	for _, s := range market.Sessions {
		if For(config.LanguageEn, "").Session(s) == "" || For(config.LanguageZh, "").Session(s) == "" {
			t.Errorf("%s has no label", s)
		}
	}
}

func TestPercent(t *testing.T) {
	for v, want := range map[float64]string{1: "100%", 0.85: "85%", 0.7: "70%", 0.5: "50%"} {
		if got := Percent(v); got != want {
			t.Errorf("Percent(%v) = %s", v, got)
		}
	}
}
