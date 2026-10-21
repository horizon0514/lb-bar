package main

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"lb-bar/feed"
)

// price formats a price: 2 decimals, 3 under 10, 4 under 1 (pennies and
// warrants).
func price(v float64) string {
	if v == 0 || math.IsNaN(v) {
		return "--"
	}
	switch a := math.Abs(v); {
	case a < 1:
		return fmt.Sprintf("%.4f", v)
	case a < 10:
		return fmt.Sprintf("%.3f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// signed is a change with its sign: +6.10, -0.25.
func signed(v float64) string {
	if v == 0 {
		return "0.00"
	}
	s := fmt.Sprintf("%.2f", math.Abs(v))
	if math.Abs(v) < 0.1 {
		s = fmt.Sprintf("%.3f", math.Abs(v))
	}
	if v > 0 {
		return "+" + s
	}
	return "-" + s
}

// pct is a rate as a signed percentage: 0.012 is +1.20%.
func pct(rate float64) string {
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return "--"
	}
	p := rate * 100
	if math.Abs(p) < 0.005 {
		return "0.00%"
	}
	return fmt.Sprintf("%+.2f%%", p)
}

// big says a large number in Chinese units: 1.23万, 4.56亿, 7.89万亿.
func big(v float64) string {
	a := math.Abs(v)
	switch {
	case v == 0:
		return "--"
	case a >= 1e12:
		return fmt.Sprintf("%.2f万亿", v/1e12)
	case a >= 1e8:
		return fmt.Sprintf("%.2f亿", v/1e8)
	case a >= 1e4:
		return fmt.Sprintf("%.2f万", v/1e4)
	}
	return fmt.Sprintf("%.0f", v)
}

// money is an amount of a currency, with thousands separators: 1,234.56.
func money(v float64) string {
	neg := v < 0
	s := fmt.Sprintf("%.2f", math.Abs(v))
	intPart, frac := s[:len(s)-3], s[len(s)-3:]
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String() + frac
	if neg {
		return "-" + out
	}
	return out
}

// signedMoney is money with its sign.
func signedMoney(v float64) string {
	if v > 0 {
		return "+" + money(v)
	}
	return money(v)
}

// shortName is a security's name for the menu bar: 腾讯控股, 阿里巴巴 (of
// 阿里巴巴-W), at most 4 characters of Chinese or 8 of Latin.
func shortName(name, symbol string) string {
	n := strings.TrimSpace(name)
	for _, suf := range []string{"-SW", "-W", "-S", "-B", "-R"} {
		n = strings.TrimSuffix(n, suf)
	}
	if n == "" {
		return feed.Code(symbol)
	}
	wide := 0
	for _, r := range n {
		if r > 0x2e80 {
			wide++
		}
	}
	limit := 8
	if wide > 0 {
		limit = 4
	}
	if utf8.RuneCountInString(n) > limit {
		n = string([]rune(n)[:limit])
	}
	return strings.TrimSpace(n)
}

// direction is +1 up, -1 down, 0 flat.
func direction(v float64) int {
	switch {
	case v > 1e-12:
		return 1
	case v < -1e-12:
		return -1
	}
	return 0
}

// clock says t in Beijing time: 10:31 today, 10月20日 10:31 this year, or
// 2025-10-20.
func clock(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	t, now = t.In(feed.Shanghai), now.In(feed.Shanghai)
	switch {
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("1月2日 15:04")
	}
	return t.Format("2006-01-02")
}

// ago says how long ago t was, for news.
func ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case t.IsZero():
		return ""
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	}
	return clock(t, now)
}

// currencySign is a currency's symbol.
func currencySign(c string) string {
	switch c {
	case "HKD":
		return "HK$"
	case "USD":
		return "$"
	case "CNY", "CNH":
		return "¥"
	case "SGD":
		return "S$"
	}
	return c + " "
}

// hidden masks an amount while amounts are hidden (⇧⌘H).
func (a *app) amount(s string) string {
	if a.cfg.HideAmounts {
		return "****"
	}
	return s
}
