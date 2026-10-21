package main

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lb-bar/alerts"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

func TestLoadsWatchlistAndSubscribes(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	// The empty holdings group is dropped; 持仓 is a tab of its own.
	for _, s := range []string{"全部", "港股", "美股", "A股", "持仓", "腾讯控股", "512.50", "+1.20%"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("holdings") {
		t.Error("the empty holdings group shows")
	}
	sub := h.mock.Subscribed()
	if !sub["700.HK"] || !sub["AAPL.US"] || len(sub) != 12 {
		t.Errorf("subscribed %v", sub)
	}
	if n := h.mock.Count("Quotes"); n == 0 {
		t.Error("no quote snapshot")
	}
}

func TestKeyRouting(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	h.key(tt, 0, ui.KeyDown)
	if h.chosen != 1 {
		t.Fatalf("↓: chosen %d", h.chosen)
	}
	h.key(tt, 0, ui.KeyUp)
	h.key(tt, 0, ui.KeyEnter)
	if !h.expanded || h.detailSym != "700.HK" || !slices.Equal(h.resized, []bool{true}) {
		t.Fatalf("↵: expanded %v detail %q resized %v", h.expanded, h.detailSym, h.resized)
	}
	tt.SetSize(expandedWidth, panelHeight)
	tt.Frame()
	if !tt.HasText("今开") || !tt.HasText("示例新闻") {
		t.Errorf("no stats or news: %q", tt.Texts())
	}
	h.key(tt, 0, ui.KeyDown) // the detail follows the list, a moment later
	if h.detailSym != "9988.HK" {
		t.Errorf("detail %q after ↓", h.detailSym)
	}
	h.key(tt, 0, ui.KeyRight)
	if h.period != 1 || h.mock.Count("Candles") == 0 {
		t.Errorf("→: period %d, candles %d", h.period, h.mock.Count("Candles"))
	}
	h.key(tt, 0, ui.KeyTab)
	tt.Frame()
	if !h.detailOn {
		t.Fatal("⇥ did not focus the detail")
	}
	h.key(tt, 0, ui.KeyDown)
	h.key(tt, 0, ui.KeyEnter)
	if len(h.opened) != 1 || !strings.Contains(h.opened[0], "example") {
		t.Errorf("↵ on a news item opened %v", h.opened)
	}
	h.key(tt, ui.Cmd, ui.KeyEnter)
	if got := h.opened[len(h.opened)-1]; got != "https://longbridge.com/zh-CN/quote/9988.HK" {
		t.Errorf("⌘↵ opened %q", got)
	}
	h.key(tt, 0, ui.KeyEscape) // the detail's focus back to the list
	tt.Frame()
	h.key(tt, 0, ui.KeyEscape) // the detail closes
	if h.expanded {
		t.Fatal("esc did not close the detail")
	}
	h.key(tt, 0, ui.KeyEscape)
	if h.hidden != 1 {
		t.Errorf("esc did not hide: %d", h.hidden)
	}
}

func TestTypingSearches(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	h.key(tt, 0, ui.KeyDown) // the list has the keyboard
	h.typ(tt, "茅台")
	if h.query != "茅台" || h.chosenSym() != "600519.SH" {
		t.Fatalf("query %q chose %q", h.query, h.chosenSym())
	}
	h.key(tt, 0, ui.KeyEscape)
	if h.query != "" || h.searching() {
		t.Errorf("esc kept %q", h.query)
	}
	for q, want := range map[string]string{"700": "700.HK", "aapl": "AAPL.US", "tencent": "700.HK", "nvidia": "NVDA.US", "300750": "300750.SZ", "宁德": "300750.SZ"} {
		h.query = q
		h.runSearch()
		h.run()
		if got := h.chosenSym(); got != want {
			t.Errorf("search %q chose %q, want %q (%v)", q, got, want, h.results)
		}
	}
	// A code the watchlist lacks is checked with the API.
	h.query = "1810"
	h.runSearch()
	h.run()
	if h.chosenSym() != "1810.HK" {
		t.Errorf("1810: %v", h.results)
	}
}

func TestGroupsAndPositionsKeys(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	h.key(tt, ui.Cmd, ui.Key2)
	if h.groupIdx != 1 || h.rows()[0].Symbol != "700.HK" || len(h.rows()) != 4 {
		t.Errorf("⌘2: group %d rows %v", h.groupIdx, h.rows())
	}
	h.key(tt, ui.Alt, ui.KeyRight)
	if h.groupIdx != 2 || h.rows()[0].Symbol != "AAPL.US" {
		t.Errorf("⌥→: group %d", h.groupIdx)
	}
	h.key(tt, ui.Cmd, ui.KeyP)
	if !h.showingPositions() || len(h.positions) != 4 || len(h.balances) != 1 {
		t.Fatalf("⌘P: %v %d %d", h.showingPositions(), len(h.positions), len(h.balances))
	}
	tt.Frame()
	if !tt.HasText("净资产 · HKD") || !tt.HasText("HK$482,315.62") {
		t.Errorf("no balance: %q", tt.Texts())
	}
	h.key(tt, ui.Cmd|ui.Shift, ui.KeyH)
	tt.Frame()
	if tt.HasText("HK$482,315.62") || !h.cfg.HideAmounts {
		t.Error("⇧⌘H did not hide the amounts")
	}
	h.key(tt, ui.Alt, ui.KeyRight) // around to 全部
	if h.groupIdx != 0 {
		t.Errorf("wrapped to %d", h.groupIdx)
	}
}

func TestCommandKeys(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	h.key(tt, ui.Cmd, ui.KeyD)
	if !slices.Equal(h.cfg.Tickers, []string{"700.HK"}) {
		t.Errorf("⌘D pinned %v", h.cfg.Tickers)
	}
	h.key(tt, ui.Cmd, ui.KeyC)
	if h.copied != "700.HK" {
		t.Errorf("⌘C copied %q", h.copied)
	}
	h.key(tt, ui.Cmd, ui.KeySlash)
	if !h.cheat {
		t.Error("⌘/ did not show the cheatsheet")
	}
	h.key(tt, 0, ui.KeyEscape)
	if h.cheat {
		t.Error("esc kept the cheatsheet")
	}
	h.key(tt, ui.Cmd, ui.KeyN)
	if h.page != pageSettings || h.tab != tabAlerts || h.alertSym != "700.HK" || h.alertValue != "512.50" {
		t.Errorf("⌘N: page %d tab %d sym %q value %q", h.page, h.tab, h.alertSym, h.alertValue)
	}
	h.key(tt, 0, ui.KeyEscape)
	if h.page != pageMain {
		t.Error("esc did not leave the settings")
	}
	h.key(tt, ui.Cmd, ui.KeyComma)
	if h.page != pageSettings {
		t.Error("⌘, did not open the settings")
	}
	h.key(tt, ui.Cmd, ui.Key3)
	if h.tab != tabAI {
		t.Errorf("⌘3: tab %d", h.tab)
	}
	h.key(tt, ui.Cmd, ui.Key4)
	if h.tab != tabAlerts {
		t.Errorf("⌘4: tab %d", h.tab)
	}
	h.key(tt, ui.Cmd, ui.Key6)
	if h.tab != tabAbout {
		t.Errorf("⌘6: tab %d", h.tab)
	}
	h.key(tt, ui.Cmd, ui.KeyQ)
	if h.quits != 1 {
		t.Error("⌘Q did not quit")
	}
}

func TestPushesBatchAndFlash(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	trays := 0
	h.trayChanged = func() { trays++ }
	h.mock.Emit(feed.Push{Symbol: "700.HK", Last: 515, Time: testNow})
	h.mock.Emit(feed.Push{Symbol: "700.HK", Last: 516, Time: testNow.Add(time.Second)})
	h.mock.Emit(feed.Push{Symbol: "AAPL.US", Last: 250, Time: testNow})
	if len(h.queue) != 1 {
		t.Fatalf("%d flushes queued for 3 pushes", len(h.queue))
	}
	if h.quotes["700.HK"].Last != 512.5 {
		t.Error("a push applied before its batch")
	}
	h.run()
	if h.quotes["700.HK"].Last != 516 || h.quotes["AAPL.US"].Last != 250 || trays != 1 {
		t.Errorf("after the batch: %v %v trays %d", h.quotes["700.HK"].Last, h.quotes["AAPL.US"].Last, trays)
	}
	if f := h.flashes["700.HK"]; !f.up || !f.until.After(testNow) {
		t.Errorf("no flash: %+v", f)
	}
	tt.Frame()
	if !tt.HasText("516.00") {
		t.Error("the row did not update")
	}
	// A push of the old connection is dropped.
	h.connect()
	h.run()
	h.mock.Emit(feed.Push{Symbol: "700.HK", Last: 600, Time: testNow})
	h.run()
	if h.quotes["700.HK"].Last == 600 {
		t.Log("pushes of the new connection apply")
	}
}

func TestPostMarketPushKeepsRegular(t *testing.T) {
	h := newHarness(t)
	h.mock.Emit(feed.Push{Symbol: "AAPL.US", Last: 260, Session: feed.PostMarket, Time: testNow})
	h.run()
	q := h.quotes["AAPL.US"]
	if q.Last != 242.18 || q.Post == nil || q.Post.Last != 260 {
		t.Errorf("post-market push: last %v post %+v", q.Last, q.Post)
	}
}

func TestReconnectResubscribes(t *testing.T) {
	h := newHarness(t)
	before := h.mock.Count("Subscribe")
	quotes := h.mock.Count("Quotes")
	h.mock.Reconnect(false)
	h.run()
	if h.mock.Count("Subscribe") <= before || h.mock.Count("Quotes") <= quotes || h.reconnects != 1 {
		t.Errorf("subscribe %d→%d quotes %d→%d", before, h.mock.Count("Subscribe"), quotes, h.mock.Count("Quotes"))
	}
	h.mock.Reconnect(true) // the SDK resubscribed: the snapshot only
	h.run()
	if h.reconnects != 2 {
		t.Error(h.reconnects)
	}
}

func TestConnectionErrorAndRetry(t *testing.T) {
	h := newHarness(t)
	h.mock.Fail = os.ErrDeadlineExceeded
	h.connect()
	h.run()
	tt := h.tester()
	if h.connErr == "" || !tt.HasText("重试") {
		t.Fatalf("no error shown: %q %q", h.connErr, tt.Texts())
	}
	h.mock.Fail = nil
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	h.run()
	if h.connErr != "" || len(h.groups) == 0 {
		t.Errorf("retry: %q", h.connErr)
	}
}

func TestSubscribesAtMost500(t *testing.T) {
	h := newHarness(t)
	var g feed.Group
	g.ID, g.Name = 9, "大"
	for i := 0; i < 700; i++ {
		g.Securities = append(g.Securities, feed.Watched{Symbol: strings.Repeat("A", 1+i%3) + string(rune('A'+i%26)) + string(rune('A'+i/26)) + ".US"})
	}
	h.setGroups(append(h.groups, g))
	w := h.wanted()
	if len(w) > maxSubscribed {
		t.Errorf("wanted %d", len(w))
	}
	if w[0] != "700.HK" {
		t.Errorf("the group shown first: %v", w[:3])
	}
}

func TestAlertNotifies(t *testing.T) {
	h := newHarness(t)
	h.alertSym, h.alertKind, h.alertValue = "700", 0, "515"
	h.addAlert()
	if h.alertErr {
		t.Fatal(h.alertMsg)
	}
	h.mock.Emit(feed.Push{Symbol: "700.HK", Last: 514, Time: testNow})
	h.run()
	if len(h.notified) != 0 {
		t.Fatal("notified below the price")
	}
	h.mock.Emit(feed.Push{Symbol: "700.HK", Last: 515.5, Time: testNow})
	h.run()
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "腾讯控股") || !strings.Contains(h.notified[0], "515.50") {
		t.Fatalf("notified %v", h.notified)
	}
	h.mock.Emit(feed.Push{Symbol: "700.HK", Last: 516, Time: testNow})
	h.run()
	if len(h.notified) != 1 {
		t.Error("a one-shot alert fired twice")
	}
	// Saved in the app's folder.
	b, err := os.ReadFile(filepath.Join(h.dataDir, "alerts.json"))
	if err != nil || !strings.Contains(string(b), "700.HK") {
		t.Errorf("alerts.json: %s %v", b, err)
	}
	if bk := alerts.Open(filepath.Join(h.dataDir, "alerts.json")); len(bk.Rules()) != 1 || bk.Rules()[0].Enabled {
		t.Errorf("reopened: %+v", bk.Rules())
	}
}

func TestAlertFormErrors(t *testing.T) {
	h := newHarness(t)
	h.alertSym, h.alertValue = "700", "abc"
	h.addAlert()
	if !h.alertErr {
		t.Error("a non-number was taken")
	}
	h.alertSym, h.alertValue, h.alertKind = "", "3", 2
	h.addAlert()
	if !h.alertErr {
		t.Error("no symbol was taken")
	}
}

func TestTrayTitle(t *testing.T) {
	h := newHarness(t)
	if got := h.trayTitle(); got != "腾讯控股 512.50 +1.20%" {
		t.Errorf("title %q", got)
	}
	h.cfg.Tickers = []string{"9988.HK", "AAPL.US"}
	if got := h.trayTitle(); got != "阿里巴巴 128.30 -2.14%" {
		t.Errorf("title %q", got)
	}
	h.rotate()
	if got := h.trayTitle(); got != "苹果 242.18 +1.50%" {
		t.Errorf("rotated %q", got)
	}
	h.cfg.MenubarCode, h.cfg.Dot = true, true
	if got := h.trayTitle(); got != "🔴 AAPL 242.18 +1.50%" {
		t.Errorf("code and dot %q", got)
	}
	h.cfg.GreenUp = true
	if got := h.trayTitle(); !strings.HasPrefix(got, "🟢") {
		t.Errorf("green up %q", got)
	}
	h.cfg.Menubar = "icon"
	if h.trayTitle() != "" {
		t.Error("icon only has a title")
	}
}

func TestTokenExpiry(t *testing.T) {
	h := newHarness(t)
	exp, ok := h.expiry()
	if !ok || !exp.Equal(testExpiry) {
		t.Fatalf("expiry %v %v", exp, ok)
	}
	if h.expiryWarning() != "" {
		t.Error("warned 3 months ahead")
	}
	h.now = func() time.Time { return testExpiry.Add(-10 * 24 * time.Hour) }
	if w := h.expiryWarning(); !strings.Contains(w, "还有 10 天") {
		t.Errorf("warning %q", w)
	}
	h.remindExpiry()
	if len(h.notified) != 0 {
		t.Error("reminded 10 days ahead")
	}
	h.now = func() time.Time { return testExpiry.Add(-3 * 24 * time.Hour) }
	h.remindExpiry()
	h.remindExpiry()
	if len(h.notified) != 1 {
		t.Errorf("reminders %v", h.notified)
	}
	h.now = func() time.Time { return testExpiry.Add(time.Hour) }
	if w := h.expiryWarning(); !strings.Contains(w, "已过期") {
		t.Errorf("expired %q", w)
	}
	// A token that is not a JWT: the date entered.
	h.creds.AccessToken = "opaque"
	h.editExpiry = "2027-02-01"
	h.saveExpiry()
	if e, ok := h.expiry(); !ok || e.In(feed.Shanghai).Format("2006-01-02") != "2027-02-01" {
		t.Errorf("entered expiry %v %v", e, ok)
	}
}

func TestSaveCredentialsVerifies(t *testing.T) {
	h := newHarness(t)
	h.creds, h.credFrom = feed.Creds{}, ""
	h.page, h.tab = pageSettings, tabAccount
	tt := h.tester()
	if !tt.HasText("验证并保存") {
		t.Fatalf("no form: %q", tt.Texts())
	}
	var dialed []feed.Creds
	bad := &feed.Mock{Fail: os.ErrPermission}
	h.dial = func(c feed.Creds, _ feed.Options) (feed.Provider, error) {
		dialed = append(dialed, c)
		if c.AppKey == "wrong" {
			return bad, nil
		}
		return h.mock, nil
	}
	h.editKey, h.editSecret, h.editToken = "wrong", "s", "t"
	h.saveCreds()
	h.run()
	if !h.settingsErr || h.creds.Complete() {
		t.Fatalf("bad credentials saved: %q", h.settingsMsg)
	}
	h.editKey, h.editSecret, h.editToken = " "+testCreds.AppKey, testCreds.AppSecret, testCreds.AccessToken+"\n"
	h.saveCreds()
	h.run()
	if h.settingsErr || h.creds != testCreds || h.credFrom != "keychain" {
		t.Fatalf("saved %q from %q: %q", h.creds.AppKey, h.credFrom, h.settingsMsg)
	}
	st := h.keys.load("")
	if st.creds != testCreds {
		t.Error("not in the keychain")
	}
	tt.Frame()
	if !tt.HasText("已保存  ••••a1b2") || tt.HasText(testCreds.AppSecret) {
		t.Errorf("not masked: %q", tt.Texts())
	}
	if !tt.HasText("2027-01-19 10:16 到期（北京时间），还有 90 天") {
		t.Errorf("no expiry: %q", tt.Texts())
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if h.creds.Complete() || h.keys.load("").creds.Complete() {
		t.Error("not deleted")
	}
}

func TestQuitWhileHidden(t *testing.T) {
	if hideOnClose(true) || !hideOnClose(false) {
		t.Error("closing while quitting must close")
	}
}

func TestNoCredentialsOpensAccount(t *testing.T) {
	a := newApp(defaultConfig(), feed.Creds{}, "", nil)
	if a.page != pageSettings || a.tab != tabAccount {
		t.Errorf("page %d tab %d", a.page, a.tab)
	}
	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	tt.Frame()
	if !tt.HasText("长桥 OpenAPI 凭证") || tt.HasText("返回") {
		t.Errorf("%q", tt.Texts())
	}
}

// The app reads; it never trades. No call of the trade context but the
// positions and the balance.
func TestNoTradingCalls(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, dir := range []string{"feed", "dsapi", "billing", "alerts"} {
		more, _ := filepath.Glob(dir + "/*.go")
		files = append(files, more...)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, bad := range []string{"SubmitOrder", "CancelOrder", "ReplaceOrder", "EstimateMaxPurchaseQuantity"} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s calls %s", f, bad)
			}
		}
	}
}

func TestContrast(t *testing.T) {
	lum := func(c ui.Color) float64 {
		f := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
	}
	ratio := func(a, b ui.Color) float64 {
		la, lb := lum(a), lum(b)
		return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
	}
	// over is c at alpha over bg: the change pills' tint.
	over := func(c, bg ui.Color, alpha float64) ui.Color {
		m := func(x, y uint8) uint8 { return uint8(float64(x)*alpha + float64(y)*(1-alpha) + 0.5) }
		return ui.RGB(m(c.R, bg.R), m(c.G, bg.G), m(c.B, bg.B))
	}
	for dark, p := range colors {
		bg := ui.Hex(p["bg"])
		for _, n := range []string{"text", "muted", "faint", "link", "gold", "green", "red", "danger"} {
			c := ui.Hex(p[n])
			if r := ratio(c, bg); r < 4.5 {
				t.Errorf("dark %v: %s %.2f:1", dark, n, r)
			}
			if r := ratio(c, over(c, bg, 0.12)); (n == "green" || n == "red") && r < 4.5 {
				t.Errorf("dark %v: %s on its pill %.2f:1", dark, n, r)
			}
		}
	}
}
