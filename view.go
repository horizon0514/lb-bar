package main

import (
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// The panel: the list alone, or, expanded (↵), with the detail on its
// right.
const (
	panelWidth, panelHeight = 560, 600
	expandedWidth           = 1080 // the list and the detail
	aiExpandedWidth         = 1400 // and the AI pane
	leftWidth               = 460  // the list, expanded
	minWidth, minHeight     = 480, 420
)

var (
	iconSearch = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>`))
	iconGear   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>`))
	iconBack   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>`))
	iconAlert  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/></svg>`))
	iconPin    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 17v5"/><path d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"/></svg>`))
	iconBell   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>`))
)

// ink is the app's palette for the window's theme.
type ink struct {
	bg, text, muted, faint, accent, link, line, hover, chosen, gold, green, red, card, danger ui.Color
	ma10, ma20                                                                                ui.Color
	up, down                                                                                  ui.Color
}

// colors are the palette's opaque colors, by name; every text color is
// WCAG AA (4.5:1) on bg, as TestContrast checks.
var colors = map[bool]map[string]string{
	false: {"bg": "#ffffff", "text": "#1f2328", "muted": "#57606a", "faint": "#6e7781", "link": "#1a4fd6",
		"gold": "#8a5a00", "green": "#137a3a", "red": "#c62828", "danger": "#c62828", "line": "#d8dce1",
		"ma10": "#6f3cc4", "ma20": "#b0287a"},
	true: {"bg": "#1c1d21", "text": "#e6e8eb", "muted": "#b0b5bd", "faint": "#8f96a0", "link": "#8ab4f8",
		"gold": "#f4c152", "green": "#5fd08a", "red": "#ff7b72", "danger": "#ff8a80", "line": "#3a3d44",
		"ma10": "#c3a2ff", "ma20": "#f28cc9"},
}

func (a *app) palette(c *ui.Context) ink {
	t := c.Theme()
	p := colors[t.Dark]
	h := func(n string) ui.Color { return ui.Hex(p[n]) }
	k := ink{bg: h("bg"), text: h("text"), muted: h("muted"), faint: h("faint"), link: h("link"), accent: t.Accent,
		gold: h("gold"), green: h("green"), red: h("red"), danger: h("danger"), line: h("line"),
		ma10: h("ma10"), ma20: h("ma20")}
	if t.Dark {
		k.hover = ui.RGB(255, 255, 255).Alpha(0.05)
		k.chosen = k.link.Alpha(0.16)
		k.card = ui.RGB(255, 255, 255).Alpha(0.045)
	} else {
		k.hover = ui.RGB(0, 0, 0).Alpha(0.04)
		k.chosen = k.link.Alpha(0.09)
		k.card = ui.Hex("#f4f5f7")
	}
	k.up, k.down = k.red, k.green // 红涨绿跌
	if a.cfg.GreenUp {
		k.up, k.down = k.green, k.red
	}
	return k
}

// tone is the color of a change.
func (k ink) tone(v float64) ui.Color {
	switch direction(v) {
	case 1:
		return k.up
	case -1:
		return k.down
	}
	return k.muted
}

func (a *app) view(c *ui.Context) {
	k := a.palette(c)
	root := c.Root().TextColor(k.text)
	if a.cfg.Translucent && c.Vibrancy() {
		root.Background(ui.Transparent)
	} else { // opaque, framed: readable over any page (DS Search 0.2.1)
		root.Background(k.bg).Radius(12).BorderWidth(1).BorderColor(k.line)
	}
	c.OnShortcut(ui.Cmd, ui.KeyQ, a.quit)
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.openSettings)
	if c.Shortcut(ui.Cmd, ui.KeySlash) || c.Shortcut(ui.Cmd|ui.Shift, ui.KeySlash) {
		a.cheat = !a.cheat
	}
	if a.cheat {
		if c.Shortcut(0, ui.KeyEscape) {
			a.cheat = false
		}
		a.cheatsheet(c, k)
		return
	}
	if a.page == pageSettings {
		if c.Shortcut(0, ui.KeyEscape) {
			a.back()
		}
		a.settings(c, k)
		return
	}
	a.keysMain(c)
	a.mainPage(c, k)
}

func (a *app) mainPage(c *ui.Context, k ink) {
	a.listOn = a.listFocus.Focused(c)
	a.detailOn = a.expanded && a.detailFocus.FocusWithin(c)
	a.chartOn = a.expanded && a.chartFocus.Focused(c)
	ui.Column(c).Fill().Children(func() {
		a.header(c, k)
		a.tabs(c, k)
		ui.Divider(c)
		a.banners(c, k)
		if a.expanded {
			panes := ui.Row(c.Key("panes")).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
			if w := panes.Bounds().W; w > 0 {
				a.panesW = w
			}
			panes.Children(func() {
				ui.Column(c.Key("left")).Width(a.leftWidth()).MinHeight(0).Children(func() { a.listPane(c, k) })
				a.splitter(c.Key("split"), k)
				a.detailPane(c.Key("detail"), k)
				if a.aiPane() {
					ui.Divider(c.Key("aidiv"))
					ui.Column(c.Key("ai")).Width(a.aiWidth()).MinHeight(0).Children(func() { a.aiPaneView(c, k) })
				}
			})
		} else {
			a.listPane(c, k)
		}
		ui.Divider(c)
		a.footer(c, k)
	})
	if a.focusDetail {
		a.focusDetail = false
		a.detailFocus.Focus()
	}
}

// header is the search field, the markets' status and the settings.
func (a *app) header(c *ui.Context, k ink) {
	a.grip(ui.Row(c.Key("header"))).Padding(14, 16, 10, 18).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconSearch).FontSize(18).TextColor(k.muted)
		in := ui.TextInputBase(c.Key("q"), &a.query).Bind(&a.input).Grow(1).FontSize(18).
			Placeholder("搜索 700、AAPL、茅台").Label("搜索")
		if in.Changed() {
			a.runSearch()
		}
		if a.focusQuery {
			a.focusQuery = false
			in.SetTextSelection(0, utf8.RuneCountInString(a.query))
		}
		if a.caretToEnd {
			a.caretToEnd = false
			n := utf8.RuneCountInString(a.query)
			in.SetTextSelection(n, n)
		}
		if in.Submitted() && a.chosenSym() != "" {
			a.expand(true)
		}
		if a.composing != "" {
			ui.Text(c, a.composing).FontSize(16).TextColor(k.muted).Underline()
		}
		if a.demo {
			badge(c, "示例数据", k.gold)
		}
		if a.connecting {
			ui.Spinner(c).Size(14, 14)
		}
		for _, m := range []string{"HK", "US", "CN"} {
			a.marketChip(c.Key("mk"+m), k, m)
		}
		ui.Box(c.Key("gear")).Padding(4).Radius(6).Cursor(ui.CursorPointer).Tooltip("设置  " + cmdKey() + ",").
			Label("设置").OnClick(a.openSettings).Children(func() {
			ui.Icon(c, iconGear).FontSize(15).TextColor(k.faint)
		})
	})
}

// marketChip is a market and its phase: a dot, green while trading.
func (a *app) marketChip(c *ui.Context, k ink, m string) {
	ph := a.phase(m)
	col := k.faint
	switch ph {
	case feed.Trading:
		col = k.green
	case feed.PreOpen, feed.PostClose, feed.Night, feed.Break:
		col = k.gold
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Tooltip(feed.MarketName(m) + " " + ph.Label(m) + " · 北京时间 " + feed.Hours(m, a.now())).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(col)
		label := feed.MarketName(m) + " " + ph.Label(m)
		if !a.expanded { // narrow: 港 美 A, the phase in the tooltip and the dot
			label = map[string]string{"HK": "港", "US": "美", "CN": "A"}[m]
			if ph != feed.Trading && ph != feed.Closed {
				label += ph.Label(m)
			}
		}
		ui.Text(c, label).FontSize(11).TextColor(k.muted).SingleLine()
	})
}

// tabs are the watchlist groups, then 持仓.
func (a *app) tabs(c *ui.Context, k ink) {
	if a.searching() {
		a.grip(ui.Row(c.Key("tabs"))).Padding(0, 16, 8, 18).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("搜索结果 %d 条", len(a.results))).FontSize(12).TextColor(k.muted).Grow(1)
			ui.Text(c, "esc 返回自选").FontSize(11).TextColor(k.faint)
		})
		return
	}
	a.grip(ui.Row(c.Key("tabs"))).Padding(0, 14, 8, 16).Gap(4).AlignItems(ui.Center).Children(func() {
		names := make([]string, 0, len(a.groups)+1)
		for _, g := range a.groups {
			names = append(names, groupName(g.Name))
		}
		if a.configured() {
			names = append(names, "持仓")
		}
		for i, n := range names {
			on := i == a.groupIdx
			bg, fg := ui.Transparent, k.muted
			if on {
				bg, fg = k.chosen, k.link
			}
			tip := ""
			if i < 9 {
				tip = cmdKey() + fmt.Sprint(i+1)
			}
			ui.Box(c.Key(fmt.Sprint("tab", i))).Padding(4, 10).Radius(7).Background(bg).Cursor(ui.CursorPointer).
				Label(n).Tooltip(tip).OnClick(func() { a.setGroup(i) }).Children(func() {
				w := 450
				if on {
					w = 600
				}
				ui.Text(c, n).FontSize(12).FontWeight(w).TextColor(fg).SingleLine()
			})
		}
		ui.Spacer(c)
		if a.loadedAt.IsZero() && a.prov != nil && len(a.groups) == 0 && a.connErr == "" {
			ui.Spinner(c).Size(12, 12)
		}
	})
}

// groupName is a watchlist group's name in Chinese, for the system ones.
func groupName(n string) string {
	switch n {
	case "all":
		return "全部"
	case "us":
		return "美股"
	case "hk":
		return "港股"
	case "cn", "a":
		return "A股"
	case "holdings":
		return "持仓股"
	}
	return n
}

// banners are the warnings over the list: the token's expiry, the
// connection's trouble.
func (a *app) banners(c *ui.Context, k ink) {
	if w := a.expiryWarning(); w != "" {
		ui.Row(c.Key("expiry")).Gap(8).Padding(7, 18).AlignItems(ui.Center).Background(k.gold.Alpha(0.12)).
			Cursor(ui.CursorPointer).OnClick(func() { a.openTab(tabAccount) }).Children(func() {
			ui.Icon(c, iconAlert).FontSize(13).TextColor(k.gold)
			ui.Text(c, w).FontSize(12).TextColor(k.gold).Grow(1).SingleLine()
			ui.Text(c, "去更换 ›").FontSize(12).TextColor(k.gold)
		})
	}
	if a.connErr != "" {
		ui.Row(c.Key("connerr")).Gap(8).Padding(7, 18).AlignItems(ui.Center).Background(k.danger.Alpha(0.08)).Children(func() {
			ui.Icon(c, iconAlert).FontSize(13).TextColor(k.danger)
			ui.Text(c, a.connErr).FontSize(12).TextColor(k.danger).Grow(1).MaxLines(2)
			ui.Button(c.Key("retry"), "重试").OnClick(a.connect)
		})
	}
}

// The left pane is at least minLeft wide, the right at least minRight.
const minLeft, minRight = 340, 420

func splitWidth(split, w float32) float32 {
	if w <= 0 {
		return leftWidth
	}
	if split <= 0 {
		split = float32(leftWidth) / expandedWidth
	}
	return max(min(split*w, w-minRight), min(minLeft, w/2))
}

func (a *app) leftWidth() float32 {
	w := a.panesW
	if w == 0 {
		w = float32(widthFor(a.cfg, true, 1<<20))
	}
	if a.aiPane() {
		w -= a.aiWidth()
	}
	return splitWidth(float32(a.cfg.Split), w)
}

// aiWidth is the AI pane's: a third of the panes, within bounds.
func (a *app) aiWidth() float32 {
	w := a.panesW
	if w == 0 {
		w = float32(widthFor(a.cfg, true, 1<<20))
	}
	return max(340, min(480, w*0.32))
}

// splitter is the line between the panes: drag it to share the width
// anew, double-click it for the default.
func (a *app) splitter(c *ui.Context, k ink) {
	line := k.line
	h := ui.Box(c).Width(7).Margin(0, -3).Cursor(ui.CursorResizeColumn).Label("分隔条")
	if h.Hovered() {
		line = k.link.Alpha(0.6)
	}
	if dx, _, ok := h.Dragged(); ok && a.panesW > 0 {
		line = k.link
		w := a.panesW // the list's and the detail's
		if a.aiPane() {
			w -= a.aiWidth()
		}
		if dx != 0 {
			a.cfg.Split = float64(splitWidth(float32(a.cfg.Split), w)+dx) / float64(w)
			a.cfg.Split = float64(splitWidth(float32(a.cfg.Split), w)) / float64(w)
		}
	}
	if _, _, ok := h.DragEnded(); ok {
		a.changed(a.cfg)
	}
	if h.DoubleClicked() {
		a.cfg.Split = 0
		a.changed(a.cfg)
	}
	h.AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(1).Grow(1).Background(line)
	})
}

// listPane is the list: the group's securities, the search's, or the
// holdings with the balances over them.
func (a *app) listPane(c *ui.Context, k ink) {
	rows := a.rows()
	if a.showingPositions() {
		a.balanceCards(c.Key("bal"), k)
	}
	n := len(rows)
	a.list.Key = func(i int) any {
		if i < n {
			return rows[i].Symbol
		}
		return i
	}
	ui.List(c.Key("list"), &a.list, n, func(i int) {
		if a.showingPositions() && i < len(a.positions) {
			a.positionRow(c.Key(rows[i].Symbol), k, i, a.positions[i])
		} else {
			a.row(c.Key(rows[i].Symbol), k, i, rows[i])
		}
	}).TrackScroll(&a.listScroll).Bind(&a.listFocus).Focusable().InputMethod(true).Label("行情列表").HandleInput(a.listInput).
		Grow(1).MinHeight(0).Padding(6, 8).Gap(2).Children(func() {
		if n == 0 {
			a.emptyList(c, k)
		}
	})
	if a.focusList {
		a.focusList = false
		a.listFocus.Focus()
	}
}

func (a *app) emptyList(c *ui.Context, k ink) {
	msg := ""
	switch {
	case !a.configured():
		msg = "先在设置 › 账号里填入长桥 OpenAPI 的凭证。"
	case a.searching() && a.resultsFor != "":
		msg = "没找到「" + a.query + "」。试试代码：港股 700、美股 AAPL、A 股 600519。"
	case a.showingPositions() && a.posBusy:
		msg = "正在读取持仓…"
	case a.showingPositions() && a.posErr != "":
		msg = "持仓读取失败：" + a.posErr
	case a.showingPositions():
		msg = "没有持仓。"
	case a.connecting || (a.prov != nil && a.loadedAt.IsZero()):
		msg = "正在连接长桥…"
	case len(a.groups) == 0 && a.prov != nil:
		msg = "自选股是空的。在长桥 App 里添加自选，或直接打字搜索。"
	}
	if msg != "" {
		notice(c, k, msg, a.posErr != "" && a.showingPositions())
	}
}

func notice(c *ui.Context, k ink, msg string, bad bool) {
	col := k.muted
	if bad {
		col = k.danger
	}
	ui.Row(c).Gap(8).Padding(24, 16).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, iconAlert).FontSize(15).TextColor(col)
		ui.Text(c, msg).FontSize(13).TextColor(col).Grow(1).LineHeight(1.45)
	})
}

// rowBackground is a row's: chosen, hovered, or tinted by a flash.
func (a *app) rowBackground(c *ui.Context, k ink, i int, sym string, row ui.Element) {
	bg := ui.Transparent
	if f, ok := a.flashes[sym]; ok {
		if left := f.until.Sub(a.now()); left > 0 {
			col := k.down
			if f.up {
				col = k.up
			}
			bg = col.Alpha(0.16 * min(1, float32(left)/float32(flashFor)))
			c.After(min(left, 50*time.Millisecond)) // fade it out
		} else {
			delete(a.flashes, sym)
		}
	}
	if i == a.chosen {
		if a.listOn {
			bg = k.chosen
		} else {
			bg = k.hover
		}
	} else if row.Hovered() {
		bg = k.hover
	}
	row.Background(bg)
}

// row is a security: its name and code, the day's sparkline, the price
// and the change.
func (a *app) row(c *ui.Context, k ink, i int, w feed.Watched) {
	q := a.quotes[w.Symbol]
	row := ui.Row(c).Padding(7, 10).Radius(9).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	a.rowBackground(c, k, i, w.Symbol, row)
	row.OnClick(func() {
		if a.chosen == i {
			a.expand(true)
		} else {
			a.choose(i)
		}
	})
	row.Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				name := w.Name
				if name == "" {
					name = a.name(w.Symbol)
				}
				ui.Text(c, name).FontSize(14).FontWeight(550).SingleLine().Shrink(1)
				if slices.Contains(a.cfg.Tickers, w.Symbol) {
					ui.Icon(c, iconPin).FontSize(11).TextColor(k.faint).Tooltip("已置顶到菜单")
				}
			})
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, feed.Code(w.Symbol)).FontSize(11).TextColor(k.faint).SingleLine()
				ui.Text(c, feed.MarketOf(w.Symbol)).FontSize(9).FontWeight(600).TextColor(k.faint).Padding(0, 4).Radius(3).Background(k.line.Alpha(0.5))
				if q != nil && q.Halted {
					ui.Text(c, "停牌").FontSize(10).TextColor(k.gold)
				}
			})
		})
		var rate float64
		if q != nil {
			_, rate = q.Change()
		}
		pts := a.sparkFor(w.Symbol)
		prev := 0.0
		if q != nil {
			prev = q.PrevClose
		}
		sparkline(c.Key("spark"), pts, feed.SessionMinutes(feed.MarketOf(w.Symbol)), prev, k.tone(rate), k.line)
		ui.Column(c).Width(88).AlignItems(ui.End).Gap(2).Children(func() {
			last := "--"
			if q != nil {
				last = price(q.Last)
			}
			ui.Text(c, last).FontSize(15).FontWeight(600).TextColor(k.tone(rate)).SingleLine()
			if label, x := a.extended(w.Symbol, q); x != nil {
				_, pr := x.Change()
				ui.Text(c, label+" "+pct(pr)).FontSize(10).TextColor(k.tone(pr)).SingleLine()
			}
		})
		changePill(c, k, q)
	})
}

// changePill is the day's change rate, tinted with its color.
func changePill(c *ui.Context, k ink, q *feed.Quote) {
	text, col := "--", k.faint
	if q != nil && q.PrevClose > 0 && q.Last > 0 {
		_, r := q.Change()
		text, col = pct(r), k.tone(r)
	}
	ui.Text(c, text).FontSize(13).FontWeight(650).TextColor(col).Background(col.Alpha(0.12)).Radius(6).Padding(4, 0).Width(78).TextAlign(ui.Center).SingleLine()
}

// sparkline draws the day's prices, with the previous close dotted.
func sparkline(c *ui.Context, pts []float64, minutes int, prev float64, col, base ui.Color) {
	pts = slices.Clone(pts)
	ui.Box(c).Size(72, 26).Draw(func(p *ui.Painter, r ui.Rect) {
		if len(pts) < 2 {
			p.Line(r.X, r.Y+r.H/2, r.X+r.W, r.Y+r.H/2, 1, base)
			return
		}
		lo, hi := pts[0], pts[0]
		for _, v := range pts {
			lo, hi = min(lo, v), max(hi, v)
		}
		if prev > 0 {
			lo, hi = min(lo, prev), max(hi, prev)
		}
		if hi-lo < 1e-9 {
			hi, lo = hi+1, lo-1
		}
		y := func(v float64) float32 { return r.Y + 1 + (r.H-2)*float32((hi-v)/(hi-lo)) }
		if prev > 0 {
			for x := r.X; x < r.X+r.W; x += 4 {
				p.Line(x, y(prev), x+2, y(prev), 1, base)
			}
		}
		// The day so far, on the whole day's width.
		n := float32(max(len(pts), minutes) - 1)
		line := make([][2]float32, len(pts))
		for i, v := range pts {
			line[i] = [2]float32{r.X + r.W*float32(i)/n, y(v)}
		}
		p.StrokePolyline(line, 1.4, col)
	})
}

func badge(c *ui.Context, text string, col ui.Color) {
	ui.Text(c, text).FontSize(10).FontWeight(600).TextColor(col).Padding(1, 6).Radius(5).Background(col.Alpha(0.13)).SingleLine()
}

// footer is the connection's state on the left, the keys on the right.
func (a *app) footer(c *ui.Context, k ink) {
	left := keys(a.cfg.Shortcut) + " 唤起"
	switch {
	case a.demo:
		left = "示例数据，非真实行情"
	case a.connecting:
		left = "正在连接长桥…"
	case a.live:
		left = fmt.Sprintf("实时推送 · %d 只", len(a.subscribed))
	case a.prov != nil:
		left = "已连接"
	}
	if a.toast != "" {
		left = a.toast
	}
	var right string
	switch {
	case a.aiOn:
		right = hint("↵", "发送", "⇥", "详情", cmdKey()+"J", "解读今日", cmdKey()+"N", "新对话", "esc", "返回")
	case a.chartOn && a.period > 0:
		right = hint("←→", "逐根", "+ −", "缩放", "esc", "清除十字线", "⇥", "AI")
	case a.chartOn:
		right = hint("←→", "逐分钟", "esc", "清除十字线", "⇥", "下一项")
	case a.detailOn && a.aiPane():
		right = hint("←→", "周期", "↑↓", "新闻", "⇥", "图表", cmdKey()+"J", "解读今日", "esc", "列表")
	case a.detailOn:
		right = hint("←→", "周期", "↑↓", "新闻", "⇥", "图表", cmdKey()+"↵", "长桥", "esc", "收起")
	case a.expanded:
		right = hint("↑↓", "选择", "⇥", "详情", "←→", "周期", cmdKey()+"J", "AI 解读", "esc", "收起")
	case a.searching():
		right = hint("↑↓", "选择", "↵", "详情", cmdKey()+"↵", "长桥", "esc", "清空")
	default:
		right = hint("↑↓", "选择", "↵", "详情", "⌥←→", "分组", cmdKey()+"/", "快捷键", "esc", "隐藏")
	}
	a.hintBar(c, k, left, right)
}

func (a *app) hintBar(c *ui.Context, k ink, left, right string) {
	a.grip(ui.Row(c.Key("hints"))).Padding(8, 16).Gap(14).AlignItems(ui.Center).FontSize(11).TextColor(k.faint).Children(func() {
		ui.Text(c, left).Grow(1).Shrink(1).SingleLine()
		ui.Text(c, right).SingleLine()
	})
}

func (a *app) cheatsheet(c *ui.Context, k ink) {
	ui.Column(c).Fill().Children(func() {
		a.grip(ui.Row(c.Key("head"))).Padding(14, 20).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "键盘快捷键").FontSize(15).FontWeight(650).Grow(1)
			ui.Text(c, "全键盘操作，鼠标可选").FontSize(12).TextColor(k.faint)
		})
		ui.Divider(c)
		ui.Scroll(c.Key("cheat")).Grow(1).Padding(10, 24, 14, 24).Gap(1).Children(func() {
			for i, g := range shortcutGroups() {
				if g[0] != "" {
					top := float32(10)
					if i == 0 {
						top = 2
					}
					ui.Text(c, g[0]).FontSize(11).FontWeight(600).TextColor(k.faint).Margin(top, 0, 3, 0)
				}
				ui.Row(c.Key(i)).Gap(14).Padding(3, 0).AlignItems(ui.Center).Children(func() {
					ui.Text(c, g[1]).FontSize(12).FontWeight(600).TextColor(k.link).Width(150).SingleLine()
					ui.Text(c, g[2]).FontSize(12).TextColor(k.text).Grow(1).SingleLine()
				})
			}
		})
		ui.Divider(c)
		a.hintBar(c, k, keys(a.cfg.Shortcut)+" 随时唤起", hint("esc", "关闭"))
	})
}

// extended is a US security's quote outside the regular session, as the
// session is: 盘前, 盘后, or 夜盘; nil while it trades, or elsewhere.
func (a *app) extended(sym string, q *feed.Quote) (string, *feed.Ext) {
	if q == nil || feed.MarketOf(sym) != "US" {
		return "", nil
	}
	ok := func(x *feed.Ext) bool { return x != nil && x.Last > 0 }
	switch a.phase("US") {
	case feed.Trading:
		return "", nil
	case feed.PreOpen:
		if ok(q.Pre) {
			return "盘前", q.Pre
		}
	case feed.Night:
		if ok(q.Night) {
			return "夜盘", q.Night
		}
	}
	if ok(q.Post) {
		return "盘后", q.Post
	}
	return "", nil
}

// grip makes el, a bar of the panel, move the panel as it is dragged
// by its background; a double click puts it back where it opens by
// default.
func (a *app) grip(el ui.Element) ui.Element {
	if _, _, ok := el.Dragged(); ok {
		if !a.moving {
			a.moving = true
			a.winDrag(dragStart)
		} else {
			a.winDrag(dragMove)
		}
	}
	if _, _, ok := el.DragEnded(); ok && a.moving {
		a.moving = false
		a.winDrag(dragEnd)
	}
	if el.DoubleClicked() {
		a.resetPosition()
	}
	return el
}

const (
	dragStart = iota
	dragMove
	dragEnd
)

// resetPosition forgets where the panel was moved to, on every screen,
// and puts it back.
func (a *app) resetPosition() {
	if len(a.cfg.PanelPos) == 0 {
		return
	}
	a.cfg.PanelPos = nil
	a.changed(a.cfg)
	a.recenter()
	a.toast = "面板已回到默认位置"
}
