// lb-bar is Longbridge quotes in the macOS menu bar: a ticker in the menu
// bar, ⌥S shows the watchlist with pushed prices, ↵ a security's chart,
// stats and news; read-only holdings; local price alerts.
package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"lb-bar/billing"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lb-bar/alerts"
	"lb-bar/feed"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const version = "0.2.0"

type shell struct {
	a        *app
	panel    *mygo.Window
	tray     *mygo.Tray
	shortcut string
	dir      string
	saving   sync.Mutex
	quitting bool

	resizeSeq int
	animating bool
	saveTimer *time.Timer

	title     string    // the menu bar's text, as set
	titleAt   time.Time // when it was set
	titleWait bool      // a later update is due
	menuLine  string
}

// hideOnClose tells whether closing the panel (esc, ⌘W) hides it instead:
// always, but while the app quits, for App.Quit gives up when a window
// refuses to close.
func hideOnClose(quitting bool) bool { return !quitting }

// quitDeadline is how long quitting may take before the process exits
// anyway.
const quitDeadline = 3 * time.Second

func (s *shell) quit() {
	if s.quitting {
		return
	}
	s.quitting = true
	s.save(s.a.cfg)
	mygo.GlobalShortcut.UnregisterAll()
	go func() {
		time.Sleep(quitDeadline)
		log.Print("quit: still running after ", quitDeadline, ", exiting")
		os.Exit(0)
	}()
	if p := s.a.prov; p != nil {
		go func() { _ = p.Close() }()
	}
	mygo.App.Quit()
}

// dialer connects to Longbridge, or, with LBBAR_DEMO=1, the example data.
func dialer(demo bool) func(feed.Creds, feed.Options) (feed.Provider, error) {
	if demo {
		return func(feed.Creds, feed.Options) (feed.Provider, error) {
			m := &feed.Mock{}
			go m.Live(context.Background(), 1200*time.Millisecond)
			return m, nil
		}
	}
	return func(c feed.Creds, o feed.Options) (feed.Provider, error) { return feed.Dial(c, o) }
}

func (s *shell) ready() {
	dir := dataDir()
	s.dir = dir
	cfg := loadConfig(dir)
	demo := os.Getenv("LBBAR_DEMO") == "1"
	ks := credStore{secrets: mygoSecrets{}, dir: dir, env: os.Getenv}
	var st credState
	if !demo {
		st = ks.load(cfg.CredStore)
	}
	a := newApp(cfg, st.creds, st.from, dialer(demo))
	a.keys, a.credErr, a.dataDir, a.demo = ks, st.err, dir, demo
	if demo {
		a.page = pageMain
	}
	a.alerts = alerts.Open(filepath.Join(dir, "alerts.json"))
	a.ledger = billing.Open(filepath.Join(dir, "usage.json"))
	a.aiKeys = aiKeyStore{secrets: mygoSecrets{}, dir: dir, env: os.Getenv}
	a.loadAIKey()
	s.a = a

	s.panel = mygo.NewWindow(mygo.WindowOptions{
		Title:       "LB Bar",
		Panel:       true,
		Hidden:      true,
		Frameless:   true,
		Transparent: true,
		Vibrancy:    vibrancy(cfg.Translucent),
		Width:       widthFor(cfg, false, expandedWidth+48),
		Height:      heightFor(cfg),
		MinWidth:    minWidth,
		MinHeight:   minHeight,
		Content:     ui.View(a.view),
	})
	s.panel.OnClose(func(e *mygo.CloseEvent) {
		if hideOnClose(s.quitting) {
			e.PreventDefault()
			s.panel.Hide()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { s.quitting = true })
	s.panel.OnBlur(s.panel.Hide)

	a.post = func(fn func()) { s.panel.Update(fn) }
	a.spawn = func(fn func()) { go fn() }
	a.later = func(d time.Duration, fn func()) { time.AfterFunc(d, func() { s.panel.Update(fn) }) }
	a.hide = s.panel.Hide
	a.quit = s.quit
	a.open = func(u string) {
		if err := mygo.Shell.OpenExternal(u); err != nil {
			log.Print("open: ", err)
		}
	}
	a.changed = func(cfg config) { go s.save(cfg) }
	a.copy = func(t string) {
		if err := mygo.Clipboard.Write(transfer.TextData(t)); err != nil {
			log.Print("copy: ", err)
		}
	}
	a.notify = func(id, title, body string) {
		go func() {
			n := mygo.NewNotification(mygo.NotificationOptions{ID: id, Title: title, Body: body, Group: "lbbar"})
			if err := n.Show(); err != nil {
				log.Print("notify: ", err)
			}
		}()
	}
	a.setShort = s.setShortcut
	a.resize = s.resize
	a.setTranslucent = func(on bool) { s.panel.SetVibrancy(vibrancy(on)) }
	a.loginItem = mygo.App.OpenAtLogin
	a.setLoginItem = mygo.App.SetOpenAtLogin
	a.trayChanged = s.trayChanged
	s.panel.OnResize(s.resized)
	mygo.App.OnNotificationClick(func(string) { s.panel.Update(s.show) })

	if err := s.setShortcut(cfg.Shortcut); err != nil {
		log.Printf("shortcut %s: %v", cfg.Shortcut, err)
		if cfg.Shortcut != fallbackShortcut && s.setShortcut(fallbackShortcut) == nil {
			a.cfg.Shortcut, a.editShortcut = fallbackShortcut, fallbackShortcut
		}
	}
	s.setUpTray()
	a.connect()
	a.refreshBalance(false)
	s.every(time.Minute, a.tick)
	s.rotateLater()
	if !mygo.App.WasOpenedAtLogin() {
		s.show()
	}
}

// every runs fn on the main thread every d.
func (s *shell) every(d time.Duration, fn func()) {
	go func() {
		for range time.Tick(d) {
			if s.quitting {
				return
			}
			s.panel.Update(fn)
		}
	}()
}

// rotateLater shows the next ticker in RotateSec.
func (s *shell) rotateLater() {
	sec := max(2, s.a.cfg.RotateSec)
	time.AfterFunc(time.Duration(sec)*time.Second, func() {
		s.panel.Update(func() {
			s.a.rotate()
			s.rotateLater()
		})
	})
}

func (s *shell) setShortcut(acc string) error {
	if err := mygo.GlobalShortcut.Register(acc, s.toggle); err != nil {
		return err
	}
	if s.shortcut != "" && s.shortcut != acc {
		mygo.GlobalShortcut.Unregister(s.shortcut)
	}
	s.shortcut = acc
	if s.tray != nil {
		s.tray.SetMenu(s.menu())
	}
	return nil
}

func (s *shell) save(cfg config) {
	s.saving.Lock()
	defer s.saving.Unlock()
	if err := writeConfig(s.dir, cfg); err != nil {
		log.Print("config: ", err)
	}
}

// show shows the panel on the display with the pointer, a fifth of the
// way down, ready to type.
func (s *shell) show() {
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	w, h := widthFor(s.a.cfg, s.a.expanded, d.WorkArea.Width), min(heightFor(s.a.cfg), d.WorkArea.Height-40)
	s.animating = true
	defer func() { s.animating = false }()
	s.panel.SetBounds(mygo.Rectangle{X: d.WorkArea.X + (d.WorkArea.Width-w)/2, Y: d.WorkArea.Y + (d.WorkArea.Height-h)/5, Width: w, Height: h})
	s.a.shown()
	s.panel.Show()
}

func (s *shell) toggle() {
	if s.panel.IsVisible() {
		s.panel.Hide()
	} else {
		s.show()
	}
}

func (s *shell) menu() *mygo.Menu {
	a := s.a
	iconOnly := a.cfg.Menubar == "icon"
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: a.trayLine(), Disabled: true},
		mygo.Separator(),
		{Label: "打开 LB Bar", Accelerator: s.shortcut, Click: func(*mygo.MenuItem, *mygo.Window) { s.show() }},
		{Label: "持仓（只读）", Click: func(*mygo.MenuItem, *mygo.Window) { a.showPositions(); s.show() }},
		{Label: "价格提醒…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openTab(tabAlerts); s.show() }},
		{Label: "设置…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openTab(tabGeneral); s.show() }},
		mygo.Separator(),
		{Label: "菜单栏只显示图标", Type: mygo.MenuItemCheckbox, Checked: iconOnly, Click: func(it *mygo.MenuItem, _ *mygo.Window) {
			a.cfg.Menubar = "ticker"
			if it.Checked {
				a.cfg.Menubar = "icon"
			}
			a.changed(a.cfg)
			s.trayChanged()
		}},
		{Label: "开机自动启动", Type: mygo.MenuItemCheckbox, Checked: mygo.App.OpenAtLogin(), Click: func(it *mygo.MenuItem, _ *mygo.Window) {
			if err := mygo.App.SetOpenAtLogin(it.Checked); err != nil {
				log.Print(err)
			}
		}},
		mygo.Separator(),
		{Label: "退出 LB Bar", Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) { s.quit() }},
	})
}

func (s *shell) setUpTray() {
	var err error
	s.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "LB Bar · 长桥行情"})
	if err != nil {
		log.Print("no tray icon: ", err)
		return
	}
	// Left and right clicks open the menu, which always has 退出.
	s.tray.SetMenu(s.menu())
	s.trayChanged()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		s.quitting = true
		mygo.GlobalShortcut.UnregisterAll()
		s.tray.Destroy()
	})
}

// trayInterval is how often the menu bar's text changes at most: pushes
// come many times a second.
const trayInterval = time.Second

// trayChanged updates the menu bar's text, at most once a trayInterval,
// and the menu's first line.
func (s *shell) trayChanged() {
	if s.tray == nil {
		return
	}
	if wait := trayInterval - time.Since(s.titleAt); wait > 0 {
		if !s.titleWait {
			s.titleWait = true
			time.AfterFunc(wait, func() {
				s.panel.Update(func() {
					s.titleWait = false
					s.trayChanged()
				})
			})
		}
		return
	}
	t := s.a.trayTitle()
	s.titleAt = time.Now()
	if t != s.title {
		s.title = t
		s.tray.SetTitle(t)
		tip := "LB Bar · " + s.a.trayLine()
		if s.a.connErr != "" {
			tip = "LB Bar · " + s.a.connErr
		}
		s.tray.SetToolTip(tip)
	}
	if line := s.a.trayLine(); line != s.menuLine {
		s.menuLine = line
		s.tray.SetMenu(s.menu())
	}
}

// trayIcon draws three rising candles, black on transparent for macOS
// to tint.
func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	ink := color.NRGBA{A: 255}
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.Set(x, y, ink)
			}
		}
	}
	// x, the body's top and bottom, the wick's top and bottom: rising.
	for _, c := range [][5]int{{3, 20, 28, 17, 32}, {14, 12, 24, 8, 28}, {25, 4, 16, 1, 20}} {
		fill(c[0]+3, c[3], c[0]+5, c[4]) // the wick
		fill(c[0], c[1], c[0]+8, c[2])   // the body
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	s := &shell{}
	mygo.App.SetName("LB Bar")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnSecondInstance(func([]string, string) { s.panel.Update(s.show) })
	mygo.App.WhenReady(s.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// dataDir is ~/Library/Application Support/LB Bar.
func dataDir() string {
	base, err := mygo.App.Path(mygo.PathAppData)
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Join(base, "LB Bar")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Print("data: ", err)
	}
	mygo.App.SetPath(mygo.PathUserData, dir)
	return dir
}

func vibrancy(on bool) mygo.Vibrancy {
	if on {
		return mygo.VibrancyPopover
	}
	return mygo.VibrancyNone
}

// widthFor is the panel's width, expanded or not, on a screen avail wide:
// as the user left it, or the default.
func widthFor(cfg config, expanded bool, avail int) int {
	w := cfg.Width
	if w == 0 {
		w = panelWidth
	}
	if expanded && cfg.NoAIPane {
		w = cfg.WideWidth
		if w == 0 {
			w = expandedWidth
		}
	} else if expanded {
		w = cfg.AIWidth
		if w == 0 {
			w = aiExpandedWidth
		}
	}
	return max(minWidth, min(w, avail-48))
}

func heightFor(cfg config) int {
	if cfg.Height == 0 {
		return panelHeight
	}
	return max(minHeight, cfg.Height)
}

// resized remembers the size the user dragged the panel's edges to.
func (s *shell) resized() {
	if s.animating || !s.panel.IsVisible() {
		return
	}
	w, h := s.panel.Size()
	cfg := &s.a.cfg
	if s.a.expanded && cfg.NoAIPane {
		cfg.WideWidth = w
	} else if s.a.expanded {
		cfg.AIWidth = w
	} else {
		cfg.Width = w
	}
	cfg.Height = h
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	c := *cfg
	s.saveTimer = time.AfterFunc(600*time.Millisecond, func() { s.save(c) })
}

// resize widens the panel for the detail, or narrows it back, about its
// centre and on its screen, in a few quick steps.
func (s *shell) resize(expanded bool) {
	s.resizeSeq++
	seq, cfg := s.resizeSeq, s.a.cfg
	s.animating = true
	go func() {
		defer func() {
			time.Sleep(80 * time.Millisecond)
			s.panel.Update(func() {
				if seq == s.resizeSeq {
					s.animating = false
				}
			})
		}()
		from := s.panel.Bounds()
		d := mygo.Screen.DisplayNearestPoint(mygo.Point{X: from.X + from.Width/2, Y: from.Y + from.Height/2})
		w := widthFor(cfg, expanded, d.WorkArea.Width)
		x := from.X + from.Width/2 - w/2
		x = max(d.WorkArea.X+8, min(x, d.WorkArea.X+d.WorkArea.Width-w-8))
		const steps = 7
		for i := 1; i <= steps; i++ {
			f := float64(i) / steps
			f = 1 - math.Pow(1-f, 3)
			lerp := func(a, b int) int { return a + int(float64(b-a)*f) }
			s.panel.Update(func() {
				if seq == s.resizeSeq {
					s.panel.SetBounds(mygo.Rectangle{X: lerp(from.X, x), Y: from.Y, Width: lerp(from.Width, w), Height: from.Height})
				}
			})
			time.Sleep(16 * time.Millisecond)
		}
	}()
}
