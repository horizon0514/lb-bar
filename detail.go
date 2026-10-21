package main

import (
	"fmt"

	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

const maxNews = 6

// detailPane is the chosen security: its quote, chart, stats and news.
func (a *app) detailPane(c *ui.Context, k ink) {
	sym := a.detailSym
	ui.Scroll(c).Bind(&a.detailFocus).Focusable().Label("详情").Grow(1).MinHeight(0).MinWidth(0).Padding(14, 20, 18, 20).Gap(14).Children(func() {
		if sym == "" {
			notice(c, k, "选一只股票，按 ↵ 看详情。", false)
			return
		}
		a.detailHead(c, k, sym)
		a.chartBlock(c.Key("chart"), k, sym)
		a.statsGrid(c.Key("stats"), k, sym)
		a.newsBlock(c.Key("news"), k, sym)
	})
}

func (a *app) detailHead(c *ui.Context, k ink, sym string) {
	q := a.quotes[sym]
	sec := a.secs[sym]
	ui.Column(c.Key("head")).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.name(sym)).FontSize(17).FontWeight(650).SingleLine().Shrink(1)
			ui.Text(c, sym).FontSize(12).TextColor(k.faint).SingleLine()
			if sec.Currency != "" {
				ui.Text(c, sec.Currency).FontSize(10).TextColor(k.faint).Padding(0, 4).Radius(3).Background(k.line.Alpha(0.5))
			}
			ui.Spacer(c)
			m := feed.MarketOf(sym)
			ui.Text(c, feed.MarketName(m)+" "+a.phase(m).Label(m)).FontSize(11).TextColor(k.muted)
			label, fg, bg := "☆ 置顶到菜单", k.muted, ui.Transparent
			if a.pinned(sym) {
				label, fg, bg = "★ 已在菜单", k.gold, k.gold.Alpha(0.12)
			}
			ui.Box(c.Key("pin")).Padding(2, 8).Radius(6).BorderWidth(1).BorderColor(k.line).Background(bg).Cursor(ui.CursorPointer).
				Label(label).Tooltip(cmdKey() + "D 置顶到菜单 / 取消").OnClick(func() { a.pinSym(sym) }).Children(func() {
				ui.Text(c, label+"  "+cmdKey()+"D").FontSize(11).TextColor(fg)
			})
		})
		if q == nil {
			ui.Text(c, "读取中…").FontSize(13).TextColor(k.muted)
			return
		}
		abs, rate := q.Change()
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			ui.Text(c, price(q.Last)).FontSize(28).FontWeight(700).TextColor(k.tone(rate))
			ui.Text(c, signed(abs)+"  "+pct(rate)).FontSize(14).FontWeight(600).TextColor(k.tone(rate)).Margin(0, 0, 4, 0)
			ui.Spacer(c)
			if !q.Time.IsZero() {
				ui.Text(c, "北京时间 "+clock(q.Time, a.now())).FontSize(11).TextColor(k.faint).Margin(0, 0, 5, 0)
			}
		})
		for _, e := range []struct {
			label string
			x     *feed.Ext
		}{{"盘前", q.Pre}, {"盘后", q.Post}, {"夜盘", q.Night}} {
			if e.x == nil || e.x.Last == 0 {
				continue
			}
			_, r := e.x.Change()
			ui.Row(c.Key(e.label)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, e.label).FontSize(11).TextColor(k.muted)
				ui.Text(c, price(e.x.Last)).FontSize(12).FontWeight(600).TextColor(k.tone(r))
				ui.Text(c, pct(r)).FontSize(12).TextColor(k.tone(r))
				ui.Text(c, clock(e.x.Time, a.now())).FontSize(11).TextColor(k.faint)
			})
		}
	})
}

// chartBlock is the period's tabs, the bar's numbers, and its chart.
func (a *app) chartBlock(c *ui.Context, k ink, sym string) {
	q := a.quotes[sym]
	prev := 0.0
	if q != nil {
		prev = q.PrevClose
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c.Key("periods")).Gap(4).AlignItems(ui.Center).Children(func() {
			for i, p := range periods {
				on := i == a.period
				bg, fg := ui.Transparent, k.muted
				if on {
					bg, fg = k.chosen, k.link
				}
				ui.Box(c.Key(p)).Padding(3, 10).Radius(6).Background(bg).Cursor(ui.CursorPointer).Label(p).
					OnClick(func() { a.setPeriod(i) }).Children(func() {
					ui.Text(c, p).FontSize(12).FontWeight(550).TextColor(fg)
				})
			}
			ui.Spacer(c)
			if a.detailBusy > 0 {
				ui.Spinner(c).Size(12, 12)
			}
			if a.period > 0 {
				ui.Box(c.Key("zout")).Padding(1, 7).Radius(5).Cursor(ui.CursorPointer).Label("更多 K 线").Tooltip(cmdKey() + "-").
					OnClick(func() { a.zoom(-1) }).Children(func() { ui.Text(c, "－").FontSize(12).TextColor(k.muted) })
				ui.Text(c, fmt.Sprintf("%d 根", a.bars())).FontSize(11).TextColor(k.muted)
				ui.Box(c.Key("zin")).Padding(1, 7).Radius(5).Cursor(ui.CursorPointer).Label("更少 K 线").Tooltip(cmdKey() + "=").
					OnClick(func() { a.zoom(1) }).Children(func() { ui.Text(c, "＋").FontSize(12).TextColor(k.muted) })
			}
			ui.Text(c, "←→ 周期 · ⇥ 图表").FontSize(10).TextColor(k.faint)
		})
		// The chart, then its numbers over it: built after the chart has
		// moved the crosshair, laid out from the bottom up.
		ui.Column(c.Key("chart")).Reverse().Gap(8).Children(func() {
			if a.period == 0 {
				a.intradayView(c.Key("intraday"), k, sym, prev)
			} else {
				a.candleView(c.Key("k"), k, sym)
			}
			a.chartInfo(c, k, sym, prev)
		})
		if a.detailErr != "" {
			ui.Text(c, a.detailErr).FontSize(11).TextColor(k.danger)
		}
	})
}

// statsGrid is the day's numbers and the valuation, two columns.
func (a *app) statsGrid(c *ui.Context, k ink, sym string) {
	q := a.quotes[sym]
	st, hasStats := a.stats[sym]
	sec := a.secs[sym]
	if q == nil {
		return
	}
	cur := currencySign(sec.Currency)
	pe := "--"
	if hasStats && st.PE > 0 {
		pe = fmt.Sprintf("%.2f", st.PE)
	} else if hasStats {
		pe = "亏损"
	}
	mcap, turn := "--", "--"
	if hasStats && st.MarketCap > 0 {
		mcap = cur + big(st.MarketCap)
	}
	if hasStats && st.TurnoverRate > 0 {
		turn = fmt.Sprintf("%.2f%%", st.TurnoverRate*100)
	}
	lot := "--"
	if sec.LotSize > 0 {
		lot = fmt.Sprintf("%d 股", sec.LotSize)
	}
	cells := [][2]string{
		{"今开", price(q.Open)}, {"昨收", price(q.PrevClose)},
		{"最高", price(q.High)}, {"最低", price(q.Low)},
		{"成交量", big(float64(q.Volume)) + "股"}, {"成交额", big(q.Turnover)},
		{"总市值", mcap}, {"市盈率 TTM", pe},
		{"换手率", turn}, {"每手", lot},
	}
	ui.Column(c).Gap(0).Radius(8).BorderWidth(1).BorderColor(k.line).Children(func() {
		for i := 0; i < len(cells); i += 2 {
			ui.Row(c.Key(i)).Padding(6, 12).Gap(16).Children(func() {
				for _, cell := range cells[i : i+2] {
					ui.Row(c.Key(cell[0])).Grow(1).Basis(0).Children(func() {
						ui.Text(c, cell[0]).FontSize(12).TextColor(k.muted).Grow(1)
						col := k.text
						if cell[0] == "最高" || cell[0] == "最低" || cell[0] == "今开" {
							col = k.tone(num(cell[1]) - q.PrevClose)
						}
						ui.Text(c, cell[1]).FontSize(12).FontWeight(550).TextColor(col).SingleLine()
					})
				}
			})
		}
	})
}

func num(s string) float64 {
	var v float64
	if _, err := fmt.Sscan(s, &v); err != nil {
		return 0
	}
	return v
}

// newsBlock is the latest news about the security.
func (a *app) newsBlock(c *ui.Context, k ink, sym string) {
	ns, asked := a.news[sym]
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "新闻").FontSize(13).FontWeight(600).Grow(1)
			ui.Link(c.Key("more"), "长桥查看更多 ›", quoteURL(sym)).FontSize(11)
		})
		switch {
		case !asked:
			ui.Text(c, "读取中…").FontSize(12).TextColor(k.faint)
		case len(ns) == 0:
			ui.Text(c, "暂无新闻").FontSize(12).TextColor(k.faint)
		}
		for i, n := range ns[:min(len(ns), maxNews)] {
			bg := ui.Transparent
			if i == a.newsChosen && a.detailOn {
				bg = k.chosen
			}
			row := ui.Column(c.Key(n.URL)).Padding(6, 8).Radius(7).Gap(2).Background(bg).Cursor(ui.CursorPointer).
				Label(n.Title).OnClick(func() { a.open(n.URL) })
			if row.Hovered() && i != a.newsChosen {
				row.Background(k.hover)
			}
			row.Children(func() {
				ui.Text(c, n.Title).FontSize(13).TextColor(k.text).MaxLines(2).LineHeight(1.4)
				ui.Text(c, ago(n.Time, a.now())).FontSize(11).TextColor(k.faint)
			})
		}
	})
}
