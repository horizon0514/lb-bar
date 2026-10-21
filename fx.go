package main

import (
	"fmt"
	"strings"
	"time"

	"lb-bar/feed"
)

// fxTable converts between currencies with Longbridge's rates, through
// USD, which they are quoted against.
type fxTable struct {
	usd map[string]float64 // what 1 of a currency is in USD
	at  time.Time
}

func newFX(rs []feed.Rate, at time.Time) *fxTable {
	t := &fxTable{usd: map[string]float64{"USD": 1}, at: at}
	for _, r := range rs {
		if r.Rate <= 0 {
			continue
		}
		switch {
		case r.To == "USD":
			t.usd[r.From] = r.Rate
		case r.From == "USD":
			t.usd[r.To] = 1 / r.Rate
		}
	}
	// Offshore and onshore yuan: either stands for the other.
	if _, ok := t.usd["CNY"]; !ok {
		if v, ok := t.usd["CNH"]; ok {
			t.usd["CNY"] = v
		}
	}
	if _, ok := t.usd["CNH"]; !ok {
		if v, ok := t.usd["CNY"]; ok {
			t.usd["CNH"] = v
		}
	}
	return t
}

// convert is v of from in to.
func (t *fxTable) convert(v float64, from, to string) (float64, bool) {
	if from == to {
		return v, true
	}
	if t == nil {
		return 0, false
	}
	f, ok1 := t.usd[from]
	g, ok2 := t.usd[to]
	if !ok1 || !ok2 || g == 0 {
		return 0, false
	}
	return v * f / g, true
}

// rate is how much of to 1 of from is.
func (t *fxTable) rate(from, to string) (float64, bool) { return t.convert(1, from, to) }

// posCurrencies are the currencies the holdings' totals can be shown in.
var posCurrencies = []string{"HKD", "USD", "CNY"}

// posCurrency is the currency of the holdings' totals.
func (a *app) posCurrency() string {
	for _, c := range posCurrencies {
		if a.cfg.PosCurrency == c {
			return c
		}
	}
	return "HKD"
}

// setPosCurrency shows the totals in cur, remembered.
func (a *app) setPosCurrency(cur string) {
	a.cfg.PosCurrency = cur
	a.changed(a.cfg)
}

// cyclePosCurrency (⇧⌘C on the holdings) goes HKD → USD → CNY.
func (a *app) cyclePosCurrency() {
	cur := a.posCurrency()
	for i, c := range posCurrencies {
		if c == cur {
			a.setPosCurrency(posCurrencies[(i+1)%len(posCurrencies)])
			return
		}
	}
}

// rateLine is the rates the totals were converted with, and when they
// were read: 1 USD = 7.8493 HKD · 1 CNY = 1.0871 HKD.
func (a *app) rateLine() string {
	to := a.posCurrency()
	if a.fx == nil {
		return ""
	}
	seen := map[string]bool{to: true}
	var parts []string
	add := func(cur string) {
		if cur == "CNH" {
			cur = "CNY"
		}
		if seen[cur] {
			return
		}
		seen[cur] = true
		if r, ok := a.fx.rate(cur, to); ok {
			parts = append(parts, fmt.Sprintf("1 %s = %s %s", cur, rateText(r), to))
		}
	}
	for _, p := range a.positions {
		add(p.Currency)
	}
	for _, b := range a.balances {
		add(b.Currency)
	}
	for _, c := range posCurrencies {
		add(c)
	}
	if len(parts) == 0 {
		return ""
	}
	return "汇率（长桥，北京时间 " + a.fx.at.In(feed.Shanghai).Format("01-02 15:04") + "）：" + strings.Join(parts, " · ")
}

func rateText(r float64) string {
	switch {
	case r >= 100:
		return fmt.Sprintf("%.2f", r)
	case r >= 1:
		return fmt.Sprintf("%.4f", r)
	}
	return fmt.Sprintf("%.5f", r)
}
