package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lb-bar/feed"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestMenuRows(t *testing.T) {
	h := newHarness(t)
	h.cfg.Tickers = nil
	rows := h.menuRows()
	if len(rows) != 1 || rows[0].sym != "" || !strings.Contains(rows[0].label, "添加常用股票") {
		t.Fatalf("no pins: %+v", rows)
	}
	h.cfg.Tickers = []string{"700.HK", "AAPL.US"}
	h.refreshQuotes(true)
	h.run()
	rows = h.menuRows()
	if len(rows) != 2 || rows[0].sym != "700.HK" || rows[1].sym != "AAPL.US" {
		t.Fatalf("rows: %+v", rows)
	}
	if !strings.Contains(rows[0].label, "腾讯控股") || !strings.Contains(rows[0].label, "512.50") || !strings.Contains(rows[0].label, "%") {
		t.Errorf("label: %q", rows[0].label)
	}
	// A new price is a new label.
	q := *h.quotes["700.HK"]
	q.Last = 520
	h.quotes["700.HK"] = &q
	if l := h.menuRows()[0].label; !strings.Contains(l, "520.00") {
		t.Errorf("after a push: %q", l)
	}
}

func TestPinLimitAndOrder(t *testing.T) {
	h := newHarness(t)
	h.cfg.Tickers = nil
	syms := []string{"700.HK", "9988.HK", "3690.HK", "1810.HK", "AAPL.US", "NVDA.US", "TSLA.US", "MSFT.US", "600519.SH"}
	for i, s := range syms {
		err := h.pin(s)
		if i < maxPins && err != "" {
			t.Fatalf("pin %d: %s", i, err)
		}
		if i == maxPins && err == "" {
			t.Fatalf("the %dth pin went in", i+1)
		}
	}
	if len(h.cfg.Tickers) != maxPins {
		t.Fatalf("%d pins", len(h.cfg.Tickers))
	}
	h.movePin(0, 1)
	if h.cfg.Tickers[0] != "9988.HK" || h.cfg.Tickers[1] != "700.HK" {
		t.Errorf("moved: %v", h.cfg.Tickers[:2])
	}
	h.movePin(0, -1) // nowhere to go
	h.unpin("700.HK")
	if h.pinned("700.HK") || len(h.cfg.Tickers) != maxPins-1 {
		t.Errorf("unpinned: %v", h.cfg.Tickers)
	}
	// From the settings' field: the first hit.
	h.editTicker = "茅台"
	h.addTicker()
	if !h.pinned("600519.SH") || h.editTicker != "" {
		t.Errorf("added from the field: %v (%q)", h.cfg.Tickers, h.settingsMsg)
	}
}

func TestTogglePinDetail(t *testing.T) {
	h := newHarness(t)
	h.cfg.Tickers = nil
	tt := h.tester()
	h.choose(0)
	sym := h.chosenSym()
	h.key(tt, ui.Cmd, ui.KeyD)
	if !h.pinned(sym) || !strings.Contains(h.toast, "置顶到菜单") {
		t.Fatalf("⌘D: %v, %q", h.cfg.Tickers, h.toast)
	}
	h.key(tt, ui.Cmd, ui.KeyD)
	if h.pinned(sym) {
		t.Errorf("⌘D again: %v", h.cfg.Tickers)
	}
	h.expand(true)
	h.run()
	tt.SetSize(expandedWidth, panelHeight)
	tt.Frame()
	if err := tt.Click("☆ 置顶到菜单"); err != nil {
		t.Fatal(err)
	}
	h.run()
	if !h.pinned(h.detailSym) {
		t.Errorf("the detail's button: %v", h.cfg.Tickers)
	}
}

func TestOpenSymbol(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	h.showPositions()
	h.run()
	h.query = "nv"
	h.openSymbol("NVDA.US")
	h.run()
	tt.Frame()
	if h.page != pageMain || !h.expanded || h.detailSym != "NVDA.US" || h.chosenSym() != "NVDA.US" || h.query != "" {
		t.Errorf("opened: page %d expanded %v detail %q chosen %q query %q", h.page, h.expanded, h.detailSym, h.chosenSym(), h.query)
	}
	// One in no group: found by its code.
	h.openSymbol("0005.HK")
	h.run()
	if h.detailSym != "0005.HK" || !h.expanded {
		t.Errorf("not in a group: detail %q", h.detailSym)
	}
	// From the settings too.
	h.openTab(tabGeneral)
	h.openSymbol("700.HK")
	h.run()
	if h.page != pageMain || h.detailSym != "700.HK" {
		t.Errorf("from the settings: page %d detail %q", h.page, h.detailSym)
	}
}

func TestPlacePanel(t *testing.T) {
	wa := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	x, y := placePanel(wa, 480, 520, panelPos{}, false)
	if x != 480 || y != 25+71 {
		t.Errorf("default: %d,%d", x, y)
	}
	x, y = placePanel(wa, 480, 520, panelPos{CX: 300, Y: 100}, true)
	if x != 60 || y != 125 {
		t.Errorf("remembered: %d,%d", x, y)
	}
	// Remembered off the screen (it got smaller): the default.
	if x, y = placePanel(wa, 480, 520, panelPos{CX: 2400, Y: 100}, true); x != 480 || y != 96 {
		t.Errorf("off the screen: %d,%d", x, y)
	}
	// Near the edge, or wider now (expanded): kept inside.
	x, y = placePanel(wa, 1000, 520, panelPos{CX: 1400, Y: 700}, true)
	if x != 440 || y != 25+875-520 {
		t.Errorf("kept in: %d,%d", x, y)
	}
	// A second screen, left of the main one.
	wb := mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1050}
	if x, y = placePanel(wb, 480, 520, panelPos{CX: 960, Y: 200}, true); x != -1200 || y != 200 {
		t.Errorf("second screen: %d,%d", x, y)
	}
}

func TestGripAndReset(t *testing.T) {
	h := newHarness(t)
	var got []int
	h.winDrag = func(p int) { got = append(got, p) }
	recentred := 0
	h.recenter = func() { recentred++ }
	tt := h.tester()
	r, ok := tt.Find("全部")
	if !ok {
		t.Fatal("no tabs")
	}
	// A drag on the tabs' row, right of the tabs.
	x, y := float32(panelWidth-60), r.Y+r.H/2
	tt.Press(x, y)
	tt.Move(x+40, y+30)
	tt.Move(x+80, y+60)
	tt.Release(x+80, y+60)
	tt.Frame()
	if len(got) < 3 || got[0] != dragStart || got[len(got)-1] != dragEnd {
		t.Fatalf("drag: %v", got)
	}
	h.cfg.PanelPos = map[string]panelPos{"Built-in 1440x900": {CX: 10, Y: 10}}
	tt.ClickAt(x, y)
	tt.ClickAt(x, y)
	tt.Frame()
	if len(h.cfg.PanelPos) != 0 || recentred != 1 || !strings.Contains(h.toast, "默认位置") {
		t.Errorf("double click: %v, recentred %d, %q", h.cfg.PanelPos, recentred, h.toast)
	}
	// And the button in the settings.
	h.cfg.PanelPos = map[string]panelPos{"x": {}}
	h.openTab(tabGeneral)
	tt.Frame()
	h.resetPosition()
	if len(h.cfg.PanelPos) != 0 || recentred != 2 {
		t.Errorf("reset: %v %d", h.cfg.PanelPos, recentred)
	}
}

func TestFX(t *testing.T) {
	fx := newFX([]feed.Rate{{From: "HKD", To: "USD", Rate: 0.128}, {From: "CNH", To: "USD", Rate: 0.14}}, testNow)
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if v, ok := fx.convert(100, "USD", "HKD"); !ok || !near(v, 100/0.128) {
		t.Errorf("USD→HKD %v %v", v, ok)
	}
	if v, ok := fx.convert(100, "HKD", "CNY"); !ok || !near(v, 100*0.128/0.14) {
		t.Errorf("HKD→CNY (by CNH) %v %v", v, ok)
	}
	if _, ok := fx.convert(1, "JPY", "HKD"); ok {
		t.Error("JPY with no rate")
	}
	var none *fxTable
	if v, ok := none.convert(5, "HKD", "HKD"); !ok || v != 5 {
		t.Error("same currency, no table")
	}
	if _, ok := none.convert(5, "HKD", "USD"); ok {
		t.Error("no table")
	}
}

func TestPositionsCurrency(t *testing.T) {
	h := newHarness(t)
	h.demo = true
	tt := h.tester()
	h.showPositions()
	h.run()
	tt.Frame()
	if h.fx == nil {
		t.Fatal("no rates")
	}
	hk, ok := h.holdingsIn("HKD")
	if !ok {
		t.Fatal("no HKD total")
	}
	us, _ := h.holdingsIn("USD")
	if r, _ := h.fx.rate("HKD", "USD"); math.Abs(us.value-hk.value*r) > 0.01 {
		t.Errorf("USD %v vs HKD %v × %v", us.value, hk.value, r)
	}
	if !tt.HasText("净资产 · HKD") || !tt.HasText("持仓市值 · HKD") {
		t.Errorf("HKD: %v", tt.Texts())
	}
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "汇率（长桥") {
		t.Error("no rate line")
	}
	h.key(tt, ui.Shift|ui.Cmd, ui.KeyC)
	if h.cfg.PosCurrency != "USD" || !tt.HasText("持仓市值 · USD") {
		t.Errorf("⇧⌘C: %q", h.cfg.PosCurrency)
	}
	if err := tt.Click("CNY"); err != nil {
		t.Fatal(err)
	}
	h.run()
	tt.Frame()
	if h.cfg.PosCurrency != "CNY" || !tt.HasText("持仓市值 · CNY") {
		t.Errorf("CNY: %q", h.cfg.PosCurrency)
	}
	// The row keeps its own currency, and shows the other.
	if !tt.HasText("HK$153,750.00") {
		t.Error("the row's own currency")
	}
	// Remembered.
	dir := t.TempDir()
	if err := writeConfig(dir, h.cfg); err != nil {
		t.Fatal(err)
	}
	if c := loadConfig(dir); c.PosCurrency != "CNY" {
		t.Errorf("loaded %q", c.PosCurrency)
	}
	// No rates: by currency.
	h.fx = nil
	tt.Frame()
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "按原币") {
		t.Error("no fallback")
	}
}

// The segmented controls take a click (they did not, by OnClick).
func TestSegmentedClick(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	h.openTab(tabGeneral)
	tt.SetSize(panelWidth+80, 1600)
	tt.Frame()
	if err := tt.Click("绿涨红跌"); err != nil {
		t.Fatal(err)
	}
	h.run()
	tt.Frame()
	if !h.cfg.GreenUp {
		t.Error("涨跌颜色 did not change")
	}
}

// TestMenuPreview draws the tray's menu as menuRows has it, for the
// README: the native menu can't be captured headless. Labelled so.
func TestMenuPreview(t *testing.T) {
	if testing.Short() {
		t.Skip("screenshots")
	}
	for _, dark := range []bool{false, true} {
		h := newHarness(t)
		h.demo = true
		h.cfg.Tickers = []string{"700.HK", "AAPL.US", "NVDA.US", "600519.SH"}
		h.refreshQuotes(true)
		h.run()
		tt := ui.NewTester(func(c *ui.Context) { menuPreview(c, h.app, dark) }, 400, 330)
		tt.SetScale(2)
		tt.SetDark(dark)
		tt.Advance(time.Second)
		tt.Frame()
		name := "menu-light.png"
		if dark {
			name = "menu-dark.png"
		}
		save(t, filepath.Join("shots", name), tt)
		if os.Getenv("LBBAR_DOCS") == "1" {
			save(t, filepath.Join("docs", "screenshots", name), tt)
		}
	}
}

func menuPreview(c *ui.Context, a *app, dark bool) {
	bg, fg, faint, line, page := ui.Hex("#f6f6f6"), ui.Hex("#1d1d1f"), ui.Hex("#8a8a8e"), ui.Hex("#dcdcdc"), ui.Hex("#e9e9ee")
	if dark {
		bg, fg, faint, line, page = ui.Hex("#2b2b2e"), ui.Hex("#ececec"), ui.Hex("#8e8e93"), ui.Hex("#48484a"), ui.Hex("#1c1c1e")
	}
	item := func(label, key string, dim bool) {
		col := fg
		if dim {
			col = faint
		}
		ui.Row(c.Key(label)).Padding(4, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(13).TextColor(col).Grow(1).SingleLine()
			if key != "" {
				ui.Text(c, key).FontSize(13).TextColor(faint)
			}
		})
	}
	sep := func(k string) {
		ui.Box(c.Key(k)).Height(1).Background(line).Margin(5, 10)
	}
	ui.Column(c).Fill().Background(page).Padding(10, 12).Gap(8).Children(func() {
		ui.Text(c, "示意图：按 App 的菜单数据渲染（原生菜单无法无界面截图）· 示例数据").FontSize(10).TextColor(faint)
		ui.Column(c.Key("menu")).Padding(5, 0).Radius(8).Background(bg).BorderWidth(1).BorderColor(line).Children(func() {
			item(a.trayLine(), "", true)
			sep("s1")
			for _, r := range a.menuRows() {
				item(r.label, "", false)
			}
			sep("s2")
			item("打开 LB Bar", "⌥S", false)
			item("设置…", "", false)
			item("退出 LB Bar", "⌘Q", false)
		})
	})
}
