package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"lb-bar/alerts"
	"lb-bar/dsapi"

	"github.com/egoist/mygo/ui"
)

// TestScreenshots renders the panel on the example data, light and dark,
// into shots/ (and docs/screenshots with LBBAR_DOCS=1).
func TestScreenshots(t *testing.T) {
	if testing.Short() {
		t.Skip("screenshots")
	}
	macHints = true
	defer func() { macHints = false }()
	shots := func(name string, w int, setup func(h *harness, tt *ui.Tester)) {
		for _, dark := range []bool{false, true} {
			h := newHarness(t)
			h.demo = true
			tt := ui.NewTester(h.view, w, panelHeight)
			tt.SetScale(2)
			tt.SetDark(dark)
			tt.Frame()
			setup(h, tt)
			h.run()
			tt.Advance(2 * time.Second)
			tt.Frame()
			suffix := "light"
			if dark {
				suffix = "dark"
			}
			save(t, filepath.Join("shots", name+"-"+suffix+".png"), tt)
			if os.Getenv("LBBAR_DOCS") == "1" {
				save(t, filepath.Join("docs", "screenshots", name+"-"+suffix+".png"), tt)
			}
		}
	}
	shots("watchlist", panelWidth, func(h *harness, tt *ui.Tester) {
		h.choose(0)
	})
	shots("detail", expandedWidth, func(h *harness, tt *ui.Tester) {
		h.cfg.NoAIPane = true
		h.choose(0)
		h.expand(true)
		h.run()
	})
	shots("kline", expandedWidth, func(h *harness, tt *ui.Tester) {
		h.cfg.NoAIPane = true
		h.choose(4)
		h.expand(true)
		h.run()
		h.setPeriod(1)
		h.run()
	})
	shots("search", panelWidth, func(h *harness, tt *ui.Tester) {
		h.typ(tt, "tencent")
	})
	shots("positions", panelWidth, func(h *harness, tt *ui.Tester) {
		h.showPositions()
		h.run()
	})
	shots("settings-account", panelWidth+80, func(h *harness, tt *ui.Tester) {
		h.demo = false // the example token's expiry shows
		h.openTab(tabAccount)
	})
	shots("settings-general", panelWidth+80, func(h *harness, tt *ui.Tester) {
		h.cfg.Tickers = []string{"700.HK", "AAPL.US"}
		h.openTab(tabGeneral)
	})
	shots("settings-alerts", panelWidth+80, func(h *harness, tt *ui.Tester) {
		_, _ = h.alerts.Add(alerts.Rule{Symbol: "700.HK", Name: "腾讯控股", Kind: alerts.Above, Value: 520})
		_, _ = h.alerts.Add(alerts.Rule{Symbol: "NVDA.US", Name: "英伟达", Kind: alerts.Fall, Value: 3, Repeat: true})
		h.openTab(tabAlerts)
	})
	shots("ai", aiExpandedWidth, func(h *harness, tt *ui.Tester) {
		demoAI(h, "")
		h.query = "700.HK"
		h.runSearch()
		h.run()
		h.expand(true)
		h.run()
		h.quickAction()
		h.run()
	})
	shots("ai-empty", aiExpandedWidth, func(h *harness, tt *ui.Tester) {
		demoAI(h, "")
		h.choose(0)
		h.expand(true)
		h.run()
	})
	shots("ai-positions", aiExpandedWidth, func(h *harness, tt *ui.Tester) {
		demoAI(h, demoPositionsAnswer)
		h.cfg.AIPositions = true
		h.query = "NVDA.US"
		h.runSearch()
		h.run()
		h.expand(true)
		h.run()
		h.askAI("我的持仓里美股占比怎么样？", "")
		h.run()
	})
	shots("settings-deepseek", panelWidth+80, func(h *harness, tt *ui.Tester) {
		demoAI(h, "")
		h.openTab(tabDeepSeek)
		h.run()
	})
	shots("settings-usage", panelWidth+80, func(h *harness, tt *ui.Tester) {
		demoAI(h, "")
		for i := 0; i < 3; i++ {
			h.query = []string{"700.HK", "AAPL.US", "NVDA.US"}[i]
			h.runSearch()
			h.run()
			h.expand(true)
			h.run()
			h.quickAction()
			h.run()
			h.ai.(*fakeAI).replies = demoReplies("")
		}
		h.openTab(tabUsage)
		h.dataDir = "~/Library/Application Support/LB Bar"
		h.run()
	})
	shots("cheatsheet", panelWidth, func(h *harness, tt *ui.Tester) {
		h.cheat = true
	})
}

const demoTodayAnswer = "**走势**：小幅高开后震荡上行，盘中最高 518.65、最低 501.84，现报 512.50（+1.20%），位于分时均价上方，成交额 116.5 亿 [1]。\n\n" +
	"**可能原因**：机构上调目标价提振情绪 [2]；南向资金本周持续净买入科技龙头 [5]。市场传言回购加速，尚无公告证实。\n\n" +
	"**关注**：518 附近的日内高点，以及 11 月 12 日的三季度业绩 [4]。"

const demoPositionsAnswer = "按现价计，你的 4 只持仓里美股（苹果、英伟达）市值约 2.09 万美元；腾讯、茅台为港元与人民币计价。英伟达浮盈约 +53.6%，是组合里盈利最多的一只；苹果浮亏 -3.7%。\n\n仅就持仓结构而言，美股集中于两只科技股，波动会比较一致。"

// demoAI is a scripted DeepSeek, for the screenshots: a search, then
// answer.
func demoAI(h *harness, answer string) {
	f := &fakeAI{replies: demoReplies(answer), results: []dsapi.Result{
		{Title: "示例：腾讯控股三季度业绩将于 11 月 12 日公布", URL: "https://example.com/q3", SiteName: "示例财经"},
		{Title: "示例：南向资金本周净买入港股科技龙头", URL: "https://example.com/south", SiteName: "示例证券报"},
	}}
	h.ai, h.aiKey, h.aiKeyFrom = f, "sk-example-000000000000", "keychain"
	h.balance = &dsapi.AccountBalance{Available: true, Infos: []dsapi.Balance{{Currency: "CNY", Total: "48.52"}}}
	h.balAt = testNow
}

func demoReplies(answer string) []dsapi.Reply {
	if answer != "" {
		return []dsapi.Reply{{Content: answer, Usage: &dsapi.Usage{PromptCacheMiss: 2300, Completion: 160}}}
	}
	return []dsapi.Reply{
		{ToolCalls: []dsapi.ToolCall{searchCall("s1", "腾讯控股 今日 上涨 原因")}, Usage: &dsapi.Usage{PromptCacheMiss: 2100, Completion: 40}},
		{Content: demoTodayAnswer, Usage: &dsapi.Usage{PromptCacheHit: 2048, PromptCacheMiss: 900, Completion: 260}},
	}
}
