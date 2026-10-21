package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lb-bar/alerts"
	"lb-bar/billing"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// TestRealAI asks DeepSeek about a stock of the real watchlist, on real
// Longbridge data: a question that wants a search, and 解读今日走势. The
// holdings stay off. Its screenshots (shots/real-ai-*) and its answers
// (shots/real-ai.md) show quotes and news only. Skipped without
// DEEPSEEK_API_KEY and the Longbridge credentials, and with -short.
func TestRealAI(t *testing.T) {
	ks := credStore{env: os.Getenv, secrets: &memSecrets{}}
	st := ks.load("")
	if !st.creds.Complete() || os.Getenv("DEEPSEEK_API_KEY") == "" || testing.Short() {
		t.Skip("set DEEPSEEK_API_KEY and the LONGPORT_* credentials to run")
	}
	macHints = true
	defer func() { macHints = false }()
	h := &harness{}
	a := newApp(defaultConfig(), st.creds, st.from, func(c feed.Creds, o feed.Options) (feed.Provider, error) { return feed.Dial(c, o) })
	h.app = a
	a.dataDir = t.TempDir()
	a.keys = ks
	a.alerts = alerts.Open(filepath.Join(a.dataDir, "alerts.json"))
	a.ledger = billing.Open(filepath.Join(a.dataDir, "usage.json"))
	a.aiKeys = aiKeyStore{secrets: &memSecrets{}, dir: a.dataDir, env: os.Getenv}
	a.loadAIKey()
	if a.aiKeyFrom != "env" || !a.aiReady() {
		t.Fatalf("no key: %q", a.aiKeyFrom)
	}
	a.spawn = func(fn func()) { fn() }
	a.later = func(_ time.Duration, fn func()) {
		h.mu.Lock()
		h.queue = append(h.queue, fn)
		h.mu.Unlock()
	}
	a.connect()
	h.run()
	defer a.disconnect()
	if a.connErr != "" || len(a.rows()) == 0 {
		t.Fatalf("connect: %q", a.connErr)
	}
	if a.cfg.AIPositions {
		t.Fatal("positions on")
	}
	var watched []string
	for _, w := range a.rows() {
		watched = append(watched, w.Symbol)
	}
	pick := func(prefer ...string) string {
		for _, p := range prefer {
			for _, w := range watched {
				if w == p {
					return w
				}
			}
		}
		return watched[0]
	}
	tt := ui.NewTester(a.view, aiExpandedWidth, panelHeight+80)
	tt.SetScale(2)
	open := func(sym string) {
		a.query = sym
		a.runSearch()
		h.run()
		a.expand(true)
		h.run()
		time.Sleep(time.Second)
		h.run()
		tt.Frame()
	}
	var report strings.Builder
	logTurn := func(label string, cv *conv) *turn {
		tr := cv.turns[len(cv.turns)-1]
		var qs []string
		for _, s := range tr.searches {
			qs = append(qs, fmt.Sprintf("%q（%d 条%s）", s.query, s.n, s.err))
		}
		t.Logf("%s %s: %d calls, %d searches %v, ≈¥%.4f, %d runes, cites %v, err %q", label, cv.symbol, tr.calls, len(tr.searches), qs, tr.cost, len([]rune(tr.answer)), cited(tr.answer, len(cv.sources)), tr.err)
		fmt.Fprintf(&report, "## %s · %s（%s）\n\n已附带：%s\n\n%d 次调用 · 搜索 %v · 约 ¥%.4f\n\n%s\n\n", label, cv.name, cv.symbol, strings.Join(cv.ctx.Parts, "、"), tr.calls, qs, tr.cost, tr.answer)
		for _, n := range cited(tr.answer, len(cv.sources)) {
			s := cv.sources[n-1]
			fmt.Fprintf(&report, "- [%d] %s · %s %s\n", n, s.Title, s.SiteName, s.URL)
		}
		report.WriteString("\n")
		return tr
	}

	// A question that wants the web.
	sym := pick("700.HK", "AAPL.US", "NVDA.US")
	open(sym)
	start := time.Now()
	if !a.askAI("最近一周有什么影响股价的公司公告或新闻？请联网搜索后回答，并标注来源。", "") {
		t.Fatal("not asked")
	}
	h.run()
	cv := a.chat()
	tr := logTurn("提问", cv)
	t.Logf("took %s", time.Since(start).Round(time.Millisecond))
	if tr.err != "" || tr.answer == "" {
		t.Fatalf("answer: %q", tr.err)
	}
	if len(tr.searches) == 0 {
		t.Error("no web_search")
	}
	if len(tr.searches) > maxSearchesPerTurn {
		t.Errorf("%d searches", len(tr.searches))
	}
	if strings.Contains(cv.ctx.Text, "持仓（") || !strings.Contains(cv.ctx.Text, "持仓：未附带") {
		t.Error("holdings in the context")
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		save(t, filepath.Join("shots", "real-ai-ask-"+map[bool]string{false: "light", true: "dark"}[dark]+".png"), tt)
	}

	// 解读今日走势, on another one.
	sym2 := pick("NVDA.US", "AAPL.US", "9988.HK", "600519.SH")
	if sym2 == sym {
		sym2 = watched[min(1, len(watched)-1)]
	}
	open(sym2)
	start = time.Now()
	tt.Key(ui.Cmd, ui.KeyJ)
	h.run()
	tt.Frame()
	cv2 := a.chat()
	if cv2 == nil || len(cv2.turns) == 0 {
		t.Fatal("⌘J asked nothing")
	}
	tr2 := logTurn("解读今日走势", cv2)
	t.Logf("took %s", time.Since(start).Round(time.Millisecond))
	if tr2.err != "" || tr2.answer == "" {
		t.Fatalf("quick action: %q", tr2.err)
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		save(t, filepath.Join("shots", "real-ai-today-"+map[bool]string{false: "light", true: "dark"}[dark]+".png"), tt)
	}
	r := a.ledger.Report()
	t.Logf("ledger: %d completions, %d searches, ≈¥%.4f in all", r.Total.Chat, r.Total.Search, r.Total.Cost)
	fmt.Fprintf(&report, "合计：%d 次调用，%d 次搜索，约 ¥%.4f（%s，北京时间 %s）\n", r.Total.Chat, r.Total.Search, r.Total.Cost,
		map[bool]string{true: "高峰价", false: "空闲价"}[billing.Peak(time.Now())], time.Now().In(feed.Shanghai).Format("2006-01-02 15:04"))
	_ = os.WriteFile(filepath.Join("shots", "real-ai.md"), []byte(report.String()), 0o644)
	a.refreshBalance(true)
	h.run()
	if _, _, ok := a.cny(); !ok {
		t.Errorf("balance: %q", a.balErr)
	}
}
