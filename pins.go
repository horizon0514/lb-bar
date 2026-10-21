package main

import (
	"fmt"
	"slices"

	"lb-bar/feed"
)

// The 常用股票: the securities the tray's menu lists with their prices,
// and the menu bar shows in turn (config.Tickers). ⌘D in the panel, or
// 设置 › 通用.

const maxPins = 8

func (a *app) pinned(sym string) bool { return slices.Contains(a.cfg.Tickers, sym) }

// pin adds sym; it says why not, when not.
func (a *app) pin(sym string) string {
	if a.pinned(sym) {
		return ""
	}
	if len(a.cfg.Tickers) >= maxPins {
		return fmt.Sprintf("常用股票最多 %d 只，先在设置里移除一只", maxPins)
	}
	a.cfg.Tickers = append(slices.Clone(a.cfg.Tickers), sym)
	a.changed(a.cfg)
	a.refreshQuotes(true)
	a.trayChanged()
	return ""
}

func (a *app) unpin(sym string) {
	if i := slices.Index(a.cfg.Tickers, sym); i >= 0 {
		a.cfg.Tickers = slices.Delete(slices.Clone(a.cfg.Tickers), i, i+1)
		a.changed(a.cfg)
		a.trayChanged()
	}
}

// movePin moves the i-th by d, up (-1) or down.
func (a *app) movePin(i, d int) {
	j := i + d
	if i < 0 || j < 0 || i >= len(a.cfg.Tickers) || j >= len(a.cfg.Tickers) {
		return
	}
	t := slices.Clone(a.cfg.Tickers)
	t[i], t[j] = t[j], t[i]
	a.cfg.Tickers = t
	a.changed(a.cfg)
	a.trayChanged()
}

// menuRow is a row of the tray's menu: a security and its price, or,
// with no sym, the hint to add some.
type menuRow struct{ sym, label string }

// menuRows are the 常用股票's rows: 腾讯控股  700   512.50   +1.20%.
func (a *app) menuRows() []menuRow {
	if len(a.cfg.Tickers) == 0 {
		return []menuRow{{label: "添加常用股票…（面板里选中按 " + cmdKey() + "D）"}}
	}
	rows := make([]menuRow, 0, len(a.cfg.Tickers))
	for _, sym := range a.cfg.Tickers {
		rows = append(rows, menuRow{sym: sym, label: a.menuLabel(sym)})
	}
	return rows
}

func (a *app) menuLabel(sym string) string {
	name := a.name(sym)
	label := name
	if code := feed.Code(sym); code != name {
		label += "  " + code
	}
	q := a.quotes[sym]
	if q == nil || q.Last == 0 {
		return label + "    --"
	}
	_, rate := q.Change()
	s := label + "    " + price(q.Last) + "   " + pct(rate)
	if a.cfg.Dot {
		s = a.dot(direction(rate)) + s
	}
	return s
}

// openSymbol shows sym's detail, from the menu: in the group shown if
// it is there, else in a group that has it, else found by a search.
func (a *app) openSymbol(sym string) {
	a.page, a.cheat = pageMain, false
	at := func() int {
		for i, w := range a.rows() {
			if w.Symbol == sym {
				return i
			}
		}
		return -1
	}
	if a.searching() {
		a.query, a.results = "", nil
	}
	i := at()
	if i < 0 {
		for g, grp := range a.groups {
			if slices.ContainsFunc(grp.Securities, func(w feed.Watched) bool { return w.Symbol == sym }) {
				a.setGroup(g)
				i = at()
				break
			}
		}
	}
	if i < 0 {
		a.query = sym
		a.runSearch()
		i = at()
	}
	if i >= 0 {
		a.chosen = i
		a.list.ScrollIntoView(i)
	}
	a.expand(true)
	if a.detailSym != sym {
		a.showDetail(sym)
	}
	a.focusDetail = true
}
