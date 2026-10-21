package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"lb-bar/alerts"

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
		h.choose(0)
		h.expand(true)
		h.run()
	})
	shots("kline", expandedWidth, func(h *harness, tt *ui.Tester) {
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
	shots("cheatsheet", panelWidth, func(h *harness, tt *ui.Tester) {
		h.cheat = true
	})
}
