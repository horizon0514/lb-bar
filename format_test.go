package main

import (
	"errors"
	"slices"
	"testing"
	"time"

	"lb-bar/feed"
)

func TestFormat(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{price(512.5), "512.50"}, {price(4.123), "4.123"}, {price(0.0512), "0.0512"}, {price(0), "--"},
		{signed(6.1), "+6.10"}, {signed(-0.025), "-0.025"}, {signed(0), "0.00"},
		{pct(0.012), "+1.20%"}, {pct(-0.02139), "-2.14%"}, {pct(0), "0.00%"},
		{big(4.66e12), "4.66万亿"}, {big(1.165e10), "116.50亿"}, {big(22_731_700), "2273.17万"}, {big(980), "980"},
		{money(482315.62), "482,315.62"}, {money(-6800), "-6,800.00"}, {money(12), "12.00"},
		{signedMoney(1830), "+1,830.00"}, {signedMoney(-74), "-74.00"},
		{shortName("阿里巴巴-W", "9988.HK"), "阿里巴巴"}, {shortName("小米集团-W", "1810.HK"), "小米集团"},
		{shortName("比亚迪股份", "1211.HK"), "比亚迪股"}, {shortName("", "AAPL.US"), "AAPL"},
		{shortName("Alphabet Inc Class A", "GOOGL.US"), "Alphabet"},
		{currencySign("HKD"), "HK$"}, {currencySign("USD"), "$"}, {currencySign("CNY"), "¥"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	now := testNow
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "刚刚"}, {now.Add(-5 * time.Minute), "5 分钟前"}, {now.Add(-3 * time.Hour), "3 小时前"},
		{time.Date(2026, 10, 1, 9, 30, 0, 0, feed.Shanghai), "10月1日 09:30"}, {time.Date(2025, 3, 2, 0, 0, 0, 0, feed.Shanghai), "2025-03-02"},
	} {
		if got := ago(c.t, now); got != c.want {
			t.Errorf("ago %v: %q, want %q", c.t, got, c.want)
		}
	}
	// Times are Beijing time, whatever the zone they come in.
	if got := clock(time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC), now); got != "10:00" {
		t.Errorf("clock %q", got)
	}
}

func TestHideAmounts(t *testing.T) {
	a := newApp(defaultConfig(), testCreds, "keychain", nil)
	if a.amount("HK$1.00") != "HK$1.00" {
		t.Error("shown")
	}
	a.cfg.HideAmounts = true
	if a.amount("HK$1.00") != "****" {
		t.Error("hidden")
	}
}

func TestCodeCandidates(t *testing.T) {
	for q, want := range map[string][]string{
		"700":       {"700.HK"},
		"00700":     {"700.HK"},
		"9988.hk":   {"9988.HK"},
		"aapl":      {"AAPL.US"},
		"BRK.B":     {"BRK.B.US"},
		"600519":    {"600519.SH"},
		"000001":    {"000001.SZ"},
		"300750":    {"300750.SZ"},
		"688981":    {"688981.SH"},
		"tsla.us":   {"TSLA.US"},
		"茅台":        nil,
		"":          nil,
		"hello wor": nil,
	} {
		if got := codeCandidates(q); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", q, got, want)
		}
	}
}

func TestSearchKnown(t *testing.T) {
	known := []feed.Watched{
		{Symbol: "700.HK", Name: "腾讯控股"}, {Symbol: "TCEHY.US", Name: "腾讯控股ADR\x00Tencent ADR"},
		{Symbol: "AAPL.US", Name: "苹果\x00Apple Inc"}, {Symbol: "APP.US", Name: "AppLovin\x00AppLovin"},
	}
	if got := searchKnown("腾讯", known, 5); len(got) < 2 || got[0].Symbol != "700.HK" {
		t.Errorf("腾讯 %v", got)
	}
	if got := searchKnown("aapl", known, 5); len(got) == 0 || got[0].Symbol != "AAPL.US" {
		t.Errorf("aapl %v", got)
	}
	if got := searchKnown("apple", known, 5); len(got) == 0 || got[0].Symbol != "AAPL.US" {
		t.Errorf("apple %v", got)
	}
	// The aliases: 茅台 without it in the watchlist.
	if got := searchKnown("茅台", nil, 5); len(got) == 0 || got[0].Symbol != "600519.SH" {
		t.Errorf("茅台 %v", got)
	}
	if got := searchKnown("zzzz", known, 5); len(got) != 0 {
		t.Errorf("zzzz %v", got)
	}
	if quoteURL("700.HK") != "https://longbridge.com/zh-CN/quote/700.HK" {
		t.Error(quoteURL("700.HK"))
	}
}

func TestKeystore(t *testing.T) {
	dir := t.TempDir()
	sec := &memSecrets{}
	ks := credStore{secrets: sec, dir: dir, env: func(string) string { return "" }}
	if st := ks.load(""); st.creds.Complete() || st.err != "" {
		t.Fatalf("empty: %+v", st)
	}
	from, warn, err := ks.save(testCreds, "")
	if err != nil || warn != "" || from != "keychain" || sec.sets != 1 {
		t.Fatalf("save: %q %q %v sets %d", from, warn, err, sec.sets)
	}
	if st := ks.load(""); st.creds != testCreds || st.from != "keychain" {
		t.Errorf("load %+v", st.from)
	}
	// One item for the three: one authorization prompt.
	if len(sec.m) != 1 {
		t.Errorf("%d keychain items", len(sec.m))
	}
	// A keychain that refuses: the file, and why.
	sec.fail = errDenied
	from, warn, err = ks.save(testCreds, "")
	if err != nil || from != "file" || warn == "" {
		t.Errorf("denied: %q %q %v", from, warn, err)
	}
	if st := ks.load("file"); st.creds != testCreds || st.from != "file" {
		t.Errorf("file %+v", st.from)
	}
	if st := ks.load(""); st.err == "" {
		t.Error("the keychain's error is not told")
	}
	sec.fail = nil
	if err := ks.remove(); err != nil {
		t.Fatal(err)
	}
	if st := ks.load("file"); st.creds.Complete() {
		t.Error("not removed")
	}
	// The environment, as the SDK names it.
	env := map[string]string{"LONGPORT_APP_KEY": "k", "LONGPORT_APP_SECRET": "s", "LONGPORT_ACCESS_TOKEN": "t"}
	ks.env = func(k string) string { return env[k] }
	if st := ks.load(""); st.from != "env" || st.creds.AppKey != "k" {
		t.Errorf("env %+v", st.from)
	}
}

func TestMask(t *testing.T) {
	if mask("abcdefghijkl") != "••••ijkl" || mask("short") != "••••" {
		t.Error(mask("abcdefghijkl"), mask("short"))
	}
}

func TestTokenExpiryParse(t *testing.T) {
	if e, ok := tokenExpiry(fakeJWT(testExpiry)); !ok || !e.Equal(testExpiry) {
		t.Errorf("%v %v", e, ok)
	}
	if e, ok := tokenExpiry("Bearer " + fakeJWT(testExpiry)); !ok || !e.Equal(testExpiry) {
		t.Errorf("bearer %v %v", e, ok)
	}
	for _, s := range []string{"", "opaque", "a.b.c", "a.e30.c"} {
		if _, ok := tokenExpiry(s); ok {
			t.Errorf("%q has an expiry", s)
		}
	}
}

func TestKeychainErr(t *testing.T) {
	if s := keychainErr(errors.New("OSStatus -25293")); s == "" {
		t.Error("empty")
	}
}
