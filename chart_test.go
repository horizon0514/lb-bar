package main

import (
	"math"
	"testing"

	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

func TestKWindowAndMA(t *testing.T) {
	if f, to := kWindow(300, 60, 0); f != 240 || to != 300 {
		t.Errorf("window %d %d", f, to)
	}
	if f, to := kWindow(300, 60, 500); f != 0 || to != 60 {
		t.Errorf("clamped %d %d", f, to)
	}
	if f, to := kWindow(20, 60, 3); f != 0 || to != 20 {
		t.Errorf("short %d %d", f, to)
	}
	cs := make([]feed.Candle, 6)
	for i := range cs {
		cs[i].Close = float64(i + 1)
	}
	m := movingAvg(cs, 5)
	if !math.IsNaN(m[3]) || m[4] != 3 || m[5] != 4 {
		t.Errorf("ma %v", m)
	}
}

// openK opens 700.HK's day K-line, wide, with the chart focused.
func openK(t *testing.T, h *harness) *ui.Tester {
	t.Helper()
	h.cfg.NoAIPane = true
	tt := ui.NewTester(h.view, expandedWidth, panelHeight+100)
	tt.Frame()
	h.query = "700.HK"
	h.runSearch()
	h.run()
	h.expand(true)
	h.run()
	h.setPeriod(1)
	h.run()
	tt.Frame()
	h.key(tt, 0, ui.KeyTab) // the detail
	h.key(tt, 0, ui.KeyTab) // the chart
	if !h.chartOn {
		t.Fatal("⇥ did not focus the chart")
	}
	return tt
}

func TestCrosshairKeys(t *testing.T) {
	h := newHarness(t)
	tt := openK(t, h)
	all := h.candles[candleKey("700.HK", 1)]
	if len(all) != candleCount {
		t.Fatalf("%d candles", len(all))
	}
	h.key(tt, 0, ui.KeyLeft)
	if h.cross.bar != len(all)-1 || !h.cross.keys {
		t.Fatalf("← put it at %d", h.cross.bar)
	}
	h.key(tt, 0, ui.KeyLeft)
	h.key(tt, 0, ui.KeyLeft)
	if h.cross.bar != len(all)-3 || h.period != 1 {
		t.Errorf("←← at %d, period %d", h.cross.bar, h.period)
	}
	// The header shows that bar.
	x := all[len(all)-3]
	if !tt.HasText(barDate(x.Time, 1)) || !tt.HasText(price(x.Close)) {
		t.Errorf("header: %q", tt.Texts())
	}
	for _, want := range []string{"MA5 ", "MA10 ", "MA20 ", "开", "收", "量", "额"} {
		if !tt.HasText(want) {
			t.Errorf("no %q", want)
		}
	}
	h.key(tt, 0, ui.KeyRight)
	if h.cross.bar != len(all)-2 {
		t.Errorf("→ at %d", h.cross.bar)
	}
	// Home, then ← goes back past the window.
	h.key(tt, 0, ui.KeyHome)
	from, _ := kWindow(len(all), h.bars(), h.kOffset)
	if h.cross.bar != from {
		t.Errorf("home %d, window from %d", h.cross.bar, from)
	}
	h.key(tt, 0, ui.KeyLeft)
	if h.kOffset != 1 || h.cross.bar != from-1 {
		t.Errorf("← at the edge: offset %d bar %d", h.kOffset, h.cross.bar)
	}
	// Zoom.
	h.key(tt, 0, ui.KeyMinus)
	if h.bars() != 120 {
		t.Errorf("- shows %d", h.bars())
	}
	h.key(tt, ui.Cmd, ui.KeyEqual)
	h.key(tt, ui.Cmd, ui.KeyEqual)
	if h.bars() != 30 {
		t.Errorf("⌘= ⌘= shows %d", h.bars())
	}
	// esc clears it, then goes back to the detail.
	h.key(tt, 0, ui.KeyEscape)
	if h.cross.bar != -1 || !h.chartOn {
		t.Errorf("esc: bar %d, chart %v", h.cross.bar, h.chartOn)
	}
	h.key(tt, 0, ui.KeyEscape)
	if h.chartOn || !h.detailOn || !h.expanded {
		t.Errorf("esc esc: chart %v detail %v", h.chartOn, h.detailOn)
	}
}

func TestCrosshairPointer(t *testing.T) {
	h := newHarness(t)
	tt := openK(t, h)
	r, ok := tt.Find("K 线图")
	if !ok {
		t.Fatal("no chart")
	}
	tt.Move(r.X+r.W*0.5, r.Y+r.H*0.3)
	tt.Frame()
	if h.cross.bar < 0 || h.cross.keys {
		t.Fatalf("hover: %+v", h.cross)
	}
	mid := h.cross.bar
	tt.Move(r.X+r.W*0.9, r.Y+r.H*0.3)
	tt.Frame()
	if h.cross.bar <= mid {
		t.Errorf("moved right: %d ≤ %d", h.cross.bar, mid)
	}
	tt.Move(r.X+r.W*0.5, r.Y-40) // off the chart
	tt.Frame()
	if h.cross.bar != -1 {
		t.Errorf("left: %d", h.cross.bar)
	}
}

func TestIntradayCrosshair(t *testing.T) {
	h := newHarness(t)
	h.cfg.NoAIPane = true
	tt := ui.NewTester(h.view, expandedWidth, panelHeight+100)
	tt.Frame()
	h.query = "700.HK"
	h.runSearch()
	h.run()
	h.expand(true)
	h.run()
	tt.Frame()
	h.key(tt, 0, ui.KeyTab)
	h.key(tt, 0, ui.KeyTab)
	pts := h.intraday["700.HK"]
	h.key(tt, 0, ui.KeyLeft)
	h.key(tt, 0, ui.KeyLeft)
	if h.cross.bar != len(pts)-2 {
		t.Fatalf("bar %d of %d", h.cross.bar, len(pts))
	}
	p := pts[len(pts)-2]
	for _, want := range []string{p.Time.In(feed.Shanghai).Format("01-02 15:04"), price(p.Price), "均价", price(p.Avg), "量"} {
		if !tt.HasText(want) {
			t.Errorf("no %q: %q", want, tt.Texts())
		}
	}
	if h.period != 0 {
		t.Error("← changed the period")
	}
}

// After a zoom in, the crosshair stays on a bar shown, the header too.
func TestCrosshairZoomClamp(t *testing.T) {
	h := newHarness(t)
	tt := openK(t, h)
	n := len(h.candles[candleKey(h.detailSym, 1)])
	h.cross = crosshair{bar: n - 100, y: -1, keys: true}
	tt.Frame()
	tt.Frame()
	from, to := kWindow(n, h.bars(), h.kOffset)
	if h.cross.bar < from || h.cross.bar >= to {
		t.Errorf("bar %d outside %d–%d", h.cross.bar, from, to)
	}
}
