package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"lb-bar/alerts"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// TestRealApp runs the panel on Longbridge with the credentials of the
// environment: the watchlist, a search, a detail. Read only. Its
// screenshots (shots/real-*) show the watchlist's quotes and never the
// account. Skipped without credentials, and with -short.
func TestRealApp(t *testing.T) {
	ks := credStore{env: os.Getenv, secrets: &memSecrets{}}
	st := ks.load("")
	if !st.creds.Complete() || testing.Short() {
		t.Skip("set LONGPORT_APP_KEY, LONGPORT_APP_SECRET and LONGPORT_ACCESS_TOKEN to run")
	}
	if exp, ok := tokenExpiry(st.creds.AccessToken); ok {
		t.Logf("access token expires %s (Beijing time)", exp.In(feed.Shanghai).Format("2006-01-02 15:04"))
	}
	macHints = true
	defer func() { macHints = false }()
	h := &harness{}
	a := newApp(defaultConfig(), st.creds, st.from, func(c feed.Creds, o feed.Options) (feed.Provider, error) { return feed.Dial(c, o) })
	h.app = a
	a.dataDir = t.TempDir()
	a.keys = ks
	a.alerts = alerts.Open(filepath.Join(a.dataDir, "alerts.json"))
	a.spawn = func(fn func()) { fn() }
	a.later = func(_ time.Duration, fn func()) {
		h.mu.Lock()
		h.queue = append(h.queue, fn)
		h.mu.Unlock()
	}
	a.connect()
	h.run()
	defer a.disconnect()
	if a.connErr != "" || len(a.groups) == 0 {
		t.Fatalf("connect: %q, %d groups", a.connErr, len(a.groups))
	}
	priced := 0
	for _, w := range a.rows() {
		if q := a.quotes[w.Symbol]; q != nil && q.Last > 0 {
			priced++
		}
	}
	t.Logf("%d groups; the first shows %d rows, %d priced; %d subscribed", len(a.groups), len(a.rows()), priced, len(a.subscribed))
	if priced == 0 {
		t.Error("no prices")
	}
	// Pushes come for a while.
	time.Sleep(4 * time.Second)
	h.run()
	t.Logf("pushes applied: live %v", a.live)

	for q, want := range map[string]string{"700": "700.HK", "AAPL": "AAPL.US", "tencent": "700.HK", "茅台": "600519.SH", "nvidia": "NVDA.US"} {
		a.query = q
		a.runSearch()
		h.run()
		if got := a.chosenSym(); got != want {
			t.Errorf("search %q chose %q, want %q", q, got, want)
		}
	}
	a.query = ""
	a.runSearch()
	h.run()

	shot := func(name string, w int, setup func()) {
		for _, dark := range []bool{false, true} {
			tt := ui.NewTester(a.view, w, panelHeight)
			tt.SetScale(2)
			tt.SetDark(dark)
			setup()
			h.run()
			tt.Frame()
			suffix := "light"
			if dark {
				suffix = "dark"
			}
			save(t, filepath.Join("shots", "real-"+name+"-"+suffix+".png"), tt)
		}
	}
	shot("watchlist", panelWidth, func() { a.choose(0) })
	shot("detail", expandedWidth, func() {
		a.cfg.NoAIPane = true
		a.query = "700"
		a.runSearch()
		h.run()
		a.expand(true)
		h.run()
		time.Sleep(time.Second)
	})
	if a.detailSym != "700.HK" || len(a.intraday["700.HK"]) == 0 || len(a.news["700.HK"]) == 0 {
		t.Errorf("detail %q: %d points, %d news", a.detailSym, len(a.intraday["700.HK"]), len(a.news["700.HK"]))
	}
	a.setPeriod(1)
	h.run()
	if len(a.candles[candleKey("700.HK", 1)]) == 0 {
		t.Error("no day candles")
	}
	if n := len(a.candles[candleKey("700.HK", 1)]); n < 250 {
		t.Errorf("%d day candles, want %d", n, candleCount)
	} else {
		t.Logf("%d day candles; 60 shown, MA5/10/20 computed", n)
	}
	shot("kline-crosshair", expandedWidth, func() {
		a.cfg.NoAIPane = true
		a.cross = crosshair{bar: len(a.candles[candleKey("700.HK", 1)]) - 9, y: -1, keys: true}
	})

	// The 常用股票 in the menu, priced from Longbridge.
	a.cfg.Tickers = []string{"700.HK", "AAPL.US", "600519.SH"}
	a.refreshQuotes(true)
	h.run()
	time.Sleep(time.Second)
	h.run()
	for _, r := range a.menuRows() {
		if a.quotes[r.sym] == nil || a.quotes[r.sym].Last == 0 {
			t.Errorf("menu row %s has no price: %q", r.sym, r.label)
		}
	}
	t.Logf("menu: %d 常用股票 rows priced", len(a.menuRows()))
	a.openSymbol("AAPL.US")
	h.run()
	if a.detailSym != "AAPL.US" {
		t.Errorf("open from the menu: %q", a.detailSym)
	}

	// The rates come with the holdings; only the rates are logged.
	a.showPositions()
	h.run()
	if a.fx == nil {
		t.Fatalf("no exchange rates: %v", a.fxErr)
	}
	for _, cur := range []string{"HKD", "CNY"} {
		if r, ok := a.fx.rate("USD", cur); !ok || r <= 0 {
			t.Errorf("1 USD in %s: %v %v", cur, r, ok)
		}
	}
	t.Log(a.rateLine())
	for _, cur := range posCurrencies {
		if _, ok := a.holdingsIn(cur); !ok && len(a.positions) > 0 {
			t.Errorf("holdings not convertible to %s", cur)
		}
	}
	t.Logf("%d positions convertible to HKD/USD/CNY (amounts not logged)", len(a.positions))
}
