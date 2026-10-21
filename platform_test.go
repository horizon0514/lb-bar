package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestHK(t *testing.T) {
	defer setPlatform(runtime.GOOS)
	setPlatform("windows")
	for in, want := range map[string]string{
		"⇧⌘C 币种":         "Ctrl+Shift+C 币种",
		"⌘D":             "Ctrl+D",
		"⌥←→":            "Alt+←→",
		"⌥←  ⌥→  /  ⌃⇥":  "Alt+←  Alt+→  /  Ctrl+Tab",
		"↵  ⇧↵":          "Enter  Shift+Enter",
		"⇧⇥ 往回":          "Shift+Tab 往回",
		"⌘1 … ⌘9":        "Ctrl+1 … Ctrl+9",
		"Ctrl+↵":         "Ctrl+Enter",
		"⌥⇧S":            "Alt+Shift+S",
		"选一只股票，按 ↵ 看详情。": "选一只股票，按 Enter 看详情。",
	} {
		if got := hk(in); got != want {
			t.Errorf("hk(%q) = %q, want %q", in, got, want)
		}
	}
	setPlatform("darwin")
	if hk("⇧⌘C") != "⇧⌘C" || cmdKey() != "⌘" || keys("Alt+S") != "⌥S" {
		t.Error("the Mac's keys changed")
	}
}

func TestPlatformDefaults(t *testing.T) {
	defer setPlatform(runtime.GOOS)
	for goos, want := range map[string][3]string{
		"darwin":  {"Alt+S", "Alt+Shift+S", "LB Bar"},
		"windows": {"Alt+Shift+S", "Alt+Shift+L", "LB Bar"},
		"linux":   {"Alt+Shift+S", "Alt+Shift+L", "lb-bar"},
	} {
		setPlatform(goos)
		if defaultShortcut != want[0] || fallbackShortcut != want[1] || dataDirName(goos) != want[2] {
			t.Errorf("%s: %s %s %s", goos, defaultShortcut, fallbackShortcut, dataDirName(goos))
		}
		if c := defaultConfig(); c.Shortcut != want[0] {
			t.Errorf("%s: config shortcut %s", goos, c.Shortcut)
		}
	}
}

func TestAnchorPanel(t *testing.T) {
	// 1920×1080, the taskbar 48 high at the bottom; the icon at the right.
	wa := mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1032}
	icon := mygo.Rectangle{X: 1780, Y: 1040, Width: 24, Height: 24}
	if x, y := anchorPanel(wa, icon, 480, 600); x != 1920-480-12 || y != 1032-600-12 {
		t.Errorf("bottom right: %d,%d", x, y)
	}
	// The taskbar at the top.
	wa = mygo.Rectangle{X: 0, Y: 48, Width: 1920, Height: 1032}
	icon = mygo.Rectangle{X: 1780, Y: 12, Width: 24, Height: 24}
	if x, y := anchorPanel(wa, icon, 480, 600); x != 1428 || y != 60 {
		t.Errorf("top right: %d,%d", x, y)
	}
	// The icon in the overflow: bottom right.
	if x, y := anchorPanel(wa, mygo.Rectangle{}, 480, 600); x != 1428 || y != 48+1032-612 {
		t.Errorf("unknown: %d,%d", x, y)
	}
	// A second screen on the left.
	wa = mygo.Rectangle{X: -1280, Y: 0, Width: 1280, Height: 760}
	if x, _ := anchorPanel(wa, mygo.Rectangle{}, 480, 600); x != -492 {
		t.Errorf("left screen: %d", x)
	}
}

func TestTrayTip(t *testing.T) {
	defer setPlatform(runtime.GOOS)
	setPlatform("windows")
	h := newHarness(t)
	h.cfg.Tickers = []string{"700.HK", "AAPL.US", "NVDA.US", "600519.SH", "9988.HK", "TSLA.US", "MSFT.US", "3690.HK"}
	h.refreshQuotes(true)
	h.run()
	tip := h.trayTip()
	if n := len([]rune(tip)); n > 127 {
		t.Errorf("%d characters", n)
	}
	if !strings.Contains(tip, "腾讯控股") || !strings.Contains(tip, "512.50") {
		t.Errorf("tip: %q", tip)
	}
	setPlatform("darwin")
	if tip := h.trayTip(); strings.Contains(tip, "512.50") {
		t.Errorf("the Mac shows the price in the bar, not the tip: %q", tip)
	}
}

// Every page speaks as the system it runs on: no ⌘ on Windows or Linux,
// nor macOS's names.
func TestPlatformTexts(t *testing.T) {
	defer setPlatform(runtime.GOOS)
	for _, goos := range []string{"windows", "linux"} {
		setPlatform(goos)
		var seen []string
		check := func(where string, tt *ui.Tester) {
			for _, s := range tt.Texts() {
				seen = append(seen, s)
				for _, bad := range []string{"⌘", "⌥", "⇧", "⌃", "macOS", "钥匙串"} {
					if strings.Contains(s, bad) {
						t.Errorf("%s %s: %q has %q", goos, where, s, bad)
					}
				}
			}
		}
		h := newHarness(t)
		h.demo = true
		tt := ui.NewTester(h.view, aiExpandedWidth, 2400)
		tt.Frame()
		check("list", tt)
		h.choose(0)
		h.expand(true)
		h.run()
		tt.Frame()
		check("detail", tt)
		h.focusChart = true
		tt.Frame()
		tt.Frame()
		check("chart", tt)
		h.showPositions()
		h.run()
		tt.Frame()
		check("positions", tt)
		h.cheat = true
		tt.Frame()
		check("cheatsheet", tt)
		h.cheat = false
		for tab := range tabNames {
			h.openTab(tab)
			h.run()
			tt.Frame()
			check(tabNames[tab], tt)
		}
		joined := strings.Join(seen, "\n")
		for _, want := range []string{"Ctrl+D", "Ctrl+Shift+C", "Ctrl+1–6", "Alt+Shift+S", platformFor(goos).store} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: no %q anywhere", goos, want)
			}
		}
	}
}

// TestPlatformShots renders the panel as on Windows and Linux, for the
// README: the keys and the names of the system. Example data.
func TestPlatformShots(t *testing.T) {
	if testing.Short() {
		t.Skip("screenshots")
	}
	defer setPlatform(runtime.GOOS)
	for _, goos := range []string{"windows", "linux"} {
		setPlatform(goos)
		for _, shot := range []struct {
			name string
			w    int
			fn   func(h *harness)
		}{
			{"detail", expandedWidth, func(h *harness) { h.cfg.NoAIPane = true; h.choose(0); h.expand(true) }},
			{"settings-general", panelWidth + 80, func(h *harness) { h.cfg.Tickers = []string{"700.HK", "AAPL.US"}; h.openTab(tabGeneral) }},
			{"cheatsheet", panelWidth, func(h *harness) { h.cheat = true }},
		} {
			h := newHarness(t)
			h.demo = true
			tt := ui.NewTester(h.view, shot.w, panelHeight)
			tt.SetScale(2)
			tt.Frame()
			shot.fn(h)
			h.run()
			tt.Advance(2 * time.Second)
			tt.Frame()
			name := shot.name + "-" + goos + ".png"
			save(t, filepath.Join("shots", name), tt)
			if os.Getenv("LBBAR_DOCS") == "1" {
				save(t, filepath.Join("docs", "screenshots", name), tt)
			}
		}
	}
}
