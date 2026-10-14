// Package market holds the watch-list symbols and the quotes shown for them.
package market

import "strings"

// Market is the exchange a symbol trades on, which is also its prefix.
type Market string

const (
	US Market = "us"
	HK Market = "hk"
	SH Market = "sh"
	SZ Market = "sz"
)

var markets = []Market{US, HK, SH, SZ}

// Symbol is a watch-list entry, written as a market prefix plus code:
// usAAPL, hk00700, sh600519.
type Symbol struct {
	Market Market
	Code   string
}

// ID is the canonical prefixed form.
func (s Symbol) ID() string { return string(s.Market) + s.Code }

// ParseID parses the canonical prefixed form only.
func ParseID(id string) (Symbol, bool) {
	for _, m := range markets {
		code, ok := strings.CutPrefix(id, string(m))
		if ok && valid(code, m) {
			return Symbol{Market: m, Code: code}, true
		}
	}
	return Symbol{}, false
}

// Parse reads user input: the prefixed form, a bare US ticker (aapl), or a
// bare Hong Kong or A-share number.
func Parse(input string) (Symbol, bool) {
	text := strings.TrimSpace(input)
	if s, ok := ParseID(text); ok {
		return s, true
	}
	if text != "" && allDigits(text) {
		switch len(text) {
		case 5:
			return ParseID("hk" + text)
		case 6:
			// Shanghai codes start with 5, 6 or 9; Shenzhen's with 0, 2 or 3.
			if strings.ContainsRune("569", rune(text[0])) {
				return ParseID("sh" + text)
			}
			return ParseID("sz" + text)
		}
		return Symbol{}, false
	}
	return ParseID("us" + strings.ToUpper(text))
}

// The prefix must be followed by an uppercase letter for US, so that a bare
// ticker like usb is not read as us + B.
func valid(code string, m Market) bool {
	if code == "" {
		return false
	}
	if m != US {
		return allDigits(code)
	}
	if !isUpper(code[0]) {
		return false
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		if !isUpper(c) && !isDigit(c) && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
