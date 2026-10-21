package main

import (
	"sort"

	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// balanceCards are the account's assets, per currency, over the
// holdings: read only.
func (a *app) balanceCards(c *ui.Context, k ink) {
	ui.Column(c).Padding(8, 14, 4, 14).Gap(8).Children(func() {
		if a.demo {
			ui.Text(c, "示例数据：以下资产和持仓都是虚构的").FontSize(11).TextColor(k.gold)
		}
		for _, b := range a.balances {
			ui.Row(c.Key(b.Currency)).Gap(12).Padding(10, 14).Radius(10).Background(k.card).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "净资产 · "+b.Currency).FontSize(11).TextColor(k.muted)
					ui.Text(c, a.amount(currencySign(b.Currency)+money(b.NetAssets))).FontSize(19).FontWeight(650).SingleLine()
				})
				ui.Column(c).AlignItems(ui.End).Gap(3).Children(func() {
					ui.Text(c, a.amount("现金 "+money(b.TotalCash))).FontSize(12).TextColor(k.muted).SingleLine()
					ui.Text(c, a.amount("购买力 "+money(b.BuyPower))).FontSize(12).TextColor(k.muted).SingleLine()
				})
			})
		}
		if ts := a.pnlTotals(); len(ts) > 0 {
			ui.Column(c.Key("pl")).Gap(0).Radius(10).BorderWidth(1).BorderColor(k.line).Padding(4, 14).Children(func() {
				ui.Row(c.Key("h")).Padding(4, 0).Children(func() {
					ui.Text(c, "持仓盈亏").FontSize(11).TextColor(k.muted).Grow(1)
					ui.Text(c, "今日").FontSize(11).TextColor(k.muted).Width(120).TextAlign(ui.End)
					ui.Text(c, "累计").FontSize(11).TextColor(k.muted).Width(120).TextAlign(ui.End)
				})
				for _, t := range ts {
					ui.Row(c.Key(t.cur)).Padding(3, 0).Children(func() {
						ui.Text(c, t.cur).FontSize(12).Grow(1)
						ui.Text(c, a.amount(signedMoney(t.today))).FontSize(12).FontWeight(600).TextColor(k.tone(t.today)).Width(120).TextAlign(ui.End).SingleLine()
						ui.Text(c, a.amount(signedMoney(t.total))).FontSize(12).FontWeight(600).TextColor(k.tone(t.total)).Width(120).TextAlign(ui.End).SingleLine()
					})
				}
			})
		}
		if a.posErr != "" && len(a.positions) > 0 {
			ui.Text(c, "刷新失败："+a.posErr).FontSize(11).TextColor(k.danger)
		}
		ui.Text(c, "只读：LB Bar 不会下单、改单或撤单 · "+cmdKey()+"R 刷新 · ⇧"+cmdKey()+"H 隐藏金额").FontSize(10).TextColor(k.faint)
	})
}

type pnlTotal struct {
	cur          string
	today, total float64
}

// pnlTotals are the holdings' P/L, summed per currency.
func (a *app) pnlTotals() []pnlTotal {
	by := map[string]*pnlTotal{}
	for _, p := range a.positions {
		_, today, total, ok := a.pnl(p)
		if !ok {
			continue
		}
		t := by[p.Currency]
		if t == nil {
			t = &pnlTotal{cur: p.Currency}
			by[p.Currency] = t
		}
		t.today += today
		t.total += total
	}
	out := make([]pnlTotal, 0, len(by))
	for _, t := range by {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].cur < out[j].cur })
	return out
}

// positionRow is a holding: name, quantity and cost; market value; today's
// and the total P/L.
func (a *app) positionRow(c *ui.Context, k ink, i int, p feed.Position) {
	row := ui.Row(c).Padding(7, 10).Radius(9).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	a.rowBackground(c, k, i, p.Symbol, row)
	row.OnClick(func() {
		if a.chosen == i {
			a.expand(true)
		} else {
			a.choose(i)
		}
	})
	value, today, total, ok := a.pnl(p)
	q := a.quotes[p.Symbol]
	sub := feed.Code(p.Symbol) + " · " + trimQty(p.Quantity) + " 股 · 成本 " + price(p.Cost)
	if ok && p.Cost > 0 && q != nil {
		sub += " · " + pct((q.Last-p.Cost)/p.Cost)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, a.name(p.Symbol)).FontSize(14).FontWeight(550).SingleLine()
			ui.Text(c, a.amount(sub)).FontSize(11).TextColor(k.faint).SingleLine()
		})
		ui.Column(c).Width(100).AlignItems(ui.End).Gap(2).Children(func() {
			last, rate := "--", 0.0
			if q != nil {
				last = price(q.Last)
				_, rate = q.Change()
			}
			ui.Text(c, last).FontSize(14).FontWeight(600).TextColor(k.tone(rate))
			mv := "--"
			if ok {
				mv = currencySign(p.Currency) + money(value)
			}
			ui.Text(c, a.amount(mv)).FontSize(11).TextColor(k.muted).SingleLine()
		})
		ui.Column(c).Width(160).AlignItems(ui.End).Gap(2).Children(func() {
			if !ok {
				ui.Text(c, "--").FontSize(13).TextColor(k.faint)
				return
			}
			ui.Text(c, a.amount("今日 "+signedMoney(today))).FontSize(13).FontWeight(600).TextColor(k.tone(today)).SingleLine()
			ui.Text(c, a.amount("累计 "+signedMoney(total))).FontSize(11).TextColor(k.tone(total)).SingleLine()
		})
	})
}

func trimQty(v float64) string {
	s := money(v)
	if len(s) > 3 && s[len(s)-3:] == ".00" {
		return s[:len(s)-3]
	}
	return s
}
