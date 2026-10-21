package main

import (
	"fmt"
	"math"
	"slices"
	"time"

	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// The detail's charts: 分时 and the K-lines, each with a volume chart
// under it and a crosshair that the pointer moves, or, the chart
// focused (⇥ from the detail), ← → bar by bar; esc clears it. Over the
// chart, the bar's numbers: the crosshair's, or the latest. The K-lines
// draw MA5, MA10 and MA20 and show 30 to 250 bars (⌘= ⌘-, or + - on the
// chart); ← at the left edge goes further back.

const (
	candleCount = 300 // K-line bars fetched, for 250 shown and their MA20
	chartH      = 250
)

var barCounts = []int{30, 60, 120, 250}

// crosshair is where the chart's crosshair is: the bar (an index into
// the series, not the window), and, from the pointer, the height it is
// at in the box (-1: from the keys, at the bar's close).
type crosshair struct {
	bar   int
	y     float32
	keys  bool    // the keys put it there: the pointer resting does not move it
	hover bool    // the pointer was over the chart
	px    float32 // where the pointer was, last frame
	py    float32
}

func (a *app) clearCross() { a.cross = crosshair{bar: -1, y: -1} }

// bars is how many K-line bars are shown.
func (a *app) bars() int {
	if slices.Contains(barCounts, a.kBars) {
		return a.kBars
	}
	return 60
}

// zoom shows fewer bars (in > 0) or more.
func (a *app) zoom(in int) {
	i := slices.Index(barCounts, a.bars()) - in
	a.kBars = barCounts[max(0, min(len(barCounts)-1, i))]
	a.kOffset = 0
}

// kWindow is the bars shown of n: [from, to), the latest unless offset
// back.
func kWindow(n, bars, offset int) (from, to int) {
	offset = max(0, min(offset, n-bars))
	to = n - offset
	return max(0, to-bars), to
}

// movingAvg is the n-bar average of the closes, NaN before n bars.
func movingAvg(cs []feed.Candle, n int) []float64 {
	out := make([]float64, len(cs))
	sum := 0.0
	for i, c := range cs {
		sum += c.Close
		if i >= n {
			sum -= cs[i-n].Close
		}
		if i >= n-1 {
			out[i] = sum / float64(n)
		} else {
			out[i] = math.NaN()
		}
	}
	return out
}

// chartGeom are a chart's parts in its box: the prices, the volumes,
// and the axis under them.
type chartGeom struct{ price, vol ui.Rect }

func geomOf(r ui.Rect) chartGeom {
	const top, bottom, gap = 10, 18, 8
	inner := ui.Rect{X: r.X + 8, Y: r.Y + top, W: r.W - 16, H: r.H - top - bottom}
	ph := (inner.H - gap) * 0.75
	return chartGeom{
		price: ui.Rect{X: inner.X, Y: inner.Y, W: inner.W, H: ph},
		vol:   ui.Rect{X: inner.X, Y: inner.Y + ph + gap, W: inner.W, H: inner.H - ph - gap},
	}
}

// track moves the crosshair with the pointer over el, as chart.Select
// does: as it comes and moves, not while it rests where it was as the
// keys move the bar. at is the bar at a height in the box.
func (a *app) track(el ui.Element, at func(x float32) int) {
	x, y, over := el.PointerPosition()
	cr := &a.cross
	if over {
		if !cr.hover || x != cr.px || y != cr.py {
			cr.bar, cr.y, cr.keys = at(x), y, false
		}
		cr.hover, cr.px, cr.py = true, x, y
	} else if cr.hover {
		cr.hover = false
		if !cr.keys {
			cr.bar, cr.y = -1, -1
		}
	}
}

// chartKeys are the focused chart's: ← → move the crosshair through
// [from, to) of n bars, back past from with more (the K-lines); esc
// clears it, or goes back to the detail.
func (a *app) chartKeys(el ui.Element, from, to, n int, more bool) {
	cr := &a.cross
	move := func(i int) { cr.bar, cr.y, cr.keys = i, -1, true }
	switch {
	case el.Shortcut(0, ui.KeyRight):
		switch {
		case cr.bar < 0:
			move(to - 1)
		case cr.bar < to-1:
			move(cr.bar + 1)
		case more && a.kOffset > 0:
			a.kOffset--
			move(min(n-1, cr.bar+1))
		}
	case el.Shortcut(0, ui.KeyLeft):
		switch {
		case cr.bar < 0:
			move(to - 1)
		case cr.bar > from:
			move(cr.bar - 1)
		case more && from > 0:
			a.kOffset++
			move(cr.bar - 1)
		}
	case el.Shortcut(0, ui.KeyHome):
		move(from)
	case el.Shortcut(0, ui.KeyEnd):
		move(to - 1)
	case el.Shortcut(0, ui.KeyEscape):
		if cr.bar >= 0 {
			a.clearCross()
		} else {
			a.focusDetail = true
		}
	case more && (el.Shortcut(0, ui.KeyEqual) || el.Shortcut(ui.Shift, ui.KeyEqual)):
		a.zoom(1)
	case more && el.Shortcut(0, ui.KeyMinus):
		a.zoom(-1)
	case el.Shortcut(0, ui.KeyTab):
		if a.aiPane() {
			a.focusAI = true
		} else {
			a.focusList = true
		}
	case el.Shortcut(ui.Shift, ui.KeyTab):
		a.focusDetail = true
	}
}

// chartBox is the box a chart draws in: focusable, a crosshair cursor.
func (a *app) chartBox(c *ui.Context, k ink, label string) ui.Element {
	box := ui.Box(c).Height(chartH).Radius(8).Background(k.card).Focusable().Bind(&a.chartFocus).
		Label(label).Cursor(ui.CursorCrosshair)
	if a.chartFocus.Focused(c) {
		box.BorderWidth(1).BorderColor(k.link.Alpha(0.6))
	}
	if a.focusChart {
		a.focusChart = false
		a.chartFocus.Focus()
	}
	return box
}

// candleView is the K-line chart, MA lines, volumes and crosshair.
func (a *app) candleView(c *ui.Context, k ink, sym string) {
	all := a.candles[candleKey(sym, a.period)]
	box := a.chartBox(c, k, "K 线图")
	if len(all) == 0 {
		box.AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c).Size(14, 14) })
		return
	}
	n := len(all)
	from, to := kWindow(n, a.bars(), a.kOffset)
	a.kOffset = n - to
	cs := all[from:to]
	ma := [3][]float64{movingAvg(all, 5), movingAvg(all, 10), movingAvg(all, 20)}
	maCol := [3]ui.Color{k.gold, k.ma10, k.ma20}
	lo, hi := cs[0].Low, cs[0].High
	var vmax int64
	for i, x := range cs {
		lo, hi = min(lo, x.Low), max(hi, x.High)
		vmax = max(vmax, x.Volume)
		for _, m := range ma {
			if v := m[from+i]; !math.IsNaN(v) {
				lo, hi = min(lo, v), max(hi, v)
			}
		}
	}
	if hi-lo < 1e-9 {
		hi, lo = hi+1, lo-1
	}
	pad := (hi - lo) * 0.04
	hi, lo = hi+pad, lo-pad
	w := box.Bounds().W
	at := func(x float32) int {
		g := geomOf(ui.Rect{W: w, H: chartH})
		if g.price.W <= 0 {
			return to - 1
		}
		i := int((x - g.price.X) / (g.price.W / float32(len(cs))))
		return from + max(0, min(len(cs)-1, i))
	}
	a.track(box, at)
	a.chartKeys(box, from, to, n, true)
	if a.cross.bar >= n {
		a.clearCross()
	}
	// Kept in the bars shown (after a zoom), the header with it; the keys
	// may have paned the window since.
	if f, t := kWindow(n, a.bars(), a.kOffset); a.cross.bar >= 0 && (a.cross.bar < f || a.cross.bar >= t) {
		a.cross.bar = max(f, min(t-1, a.cross.bar))
	}
	cr := a.cross
	period := a.period
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		g := geomOf(r)
		pr := g.price
		y := func(v float64) float32 { return pr.Y + pr.H*float32((hi-v)/(hi-lo)) }
		for _, f := range []float32{0, 0.5, 1} {
			p.Line(pr.X, pr.Y+pr.H*f, pr.X+pr.W, pr.Y+pr.H*f, 1, k.line.Alpha(0.6))
		}
		p.Line(g.vol.X, g.vol.Y+g.vol.H, g.vol.X+g.vol.W, g.vol.Y+g.vol.H, 1, k.line.Alpha(0.6))
		step := pr.W / float32(len(cs))
		bw := max(1, step*0.62)
		cx := func(i int) float32 { return pr.X + step*(float32(i-from)+0.5) }
		for i, x := range cs {
			col := k.tone(x.Close - x.Open)
			if x.Close == x.Open {
				col = k.muted
			}
			xx := cx(from + i)
			p.Line(xx, y(x.High), xx, y(x.Low), 1, col)
			top, bot := y(max(x.Open, x.Close)), y(min(x.Open, x.Close))
			p.Fill(ui.Rect{X: xx - bw/2, Y: top, W: bw, H: max(1, bot-top)}, col, 0)
			if vmax > 0 {
				vh := g.vol.H * float32(x.Volume) / float32(vmax)
				p.Fill(ui.Rect{X: xx - bw/2, Y: g.vol.Y + g.vol.H - vh, W: bw, H: max(1, vh)}, col.Alpha(0.55), 0)
			}
		}
		for j, m := range ma {
			var line [][2]float32
			for i := from; i < to; i++ {
				if !math.IsNaN(m[i]) {
					line = append(line, [2]float32{cx(i), y(m[i])})
				}
			}
			if len(line) > 1 {
				p.StrokePolyline(line, 1.1, maCol[j])
			}
		}
		p.Text(pr.X+2, pr.Y+1, price(hi), 10, k.faint)
		p.Text(pr.X+2, pr.Y+pr.H-12, price(lo), 10, k.faint)
		base := r.Y + r.H - 15
		p.Text(pr.X, base, barDate(cs[0].Time, period), 10, k.faint)
		last := barDate(cs[len(cs)-1].Time, period)
		p.Text(pr.X+pr.W-textW(last, 10), base, last, 10, k.faint)
		if cr.bar < 0 {
			return
		}
		x := cx(cr.bar)
		v := all[cr.bar].Close
		yy := y(v)
		if cr.y >= 0 && cr.y >= pr.Y-r.Y && cr.y <= pr.Y-r.Y+pr.H {
			yy = r.Y + cr.y
			v = hi - float64((yy-pr.Y)/pr.H)*(hi-lo)
		}
		crossLines(p, k, r, g, x, yy, price(v), barDate(all[cr.bar].Time, period))
	})
}

// intradayView is the day: the price, its average, the previous close
// dotted, the minutes' volumes, and the crosshair.
func (a *app) intradayView(c *ui.Context, k ink, sym string, prev float64) {
	pts := a.intraday[sym]
	minutes := feed.SessionMinutes(feed.MarketOf(sym))
	box := a.chartBox(c, k, "分时图")
	if len(pts) < 2 {
		box.AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "分时读取中，或今天还没有成交").FontSize(12).TextColor(k.faint)
		})
		return
	}
	pts = slices.Clone(pts)
	n := len(pts)
	span := float32(max(n, minutes) - 1) // the day so far, on the day's width
	lo, hi := pts[0].Price, pts[0].Price
	var vmax int64
	for _, p := range pts {
		lo, hi = min(lo, p.Price), max(hi, p.Price)
		vmax = max(vmax, p.Volume)
	}
	last := pts[n-1].Price
	col := k.tone(last - prev)
	if prev > 0 {
		// Symmetric about the previous close, as quote apps draw it.
		d := max(hi-prev, prev-lo)
		lo, hi = prev-d, prev+d
	}
	if hi-lo < 1e-9 {
		hi, lo = hi+1, lo-1
	}
	w := box.Bounds().W
	a.track(box, func(x float32) int {
		g := geomOf(ui.Rect{W: w, H: chartH})
		if g.price.W <= 0 {
			return n - 1
		}
		i := int(math.Round(float64((x - g.price.X) / g.price.W * span)))
		return max(0, min(n-1, i))
	})
	a.chartKeys(box, 0, n, n, false)
	if a.cross.bar >= n {
		a.clearCross()
	}
	cr := a.cross
	first, end := pts[0].Time, pts[n-1].Time
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		g := geomOf(r)
		pr := g.price
		y := func(v float64) float32 { return pr.Y + pr.H*float32((hi-v)/(hi-lo)) }
		xAt := func(i int) float32 { return pr.X + pr.W*float32(i)/span }
		for _, f := range []float32{0, 0.5, 1} {
			p.Line(pr.X, pr.Y+pr.H*f, pr.X+pr.W, pr.Y+pr.H*f, 1, k.line.Alpha(0.6))
		}
		p.Line(g.vol.X, g.vol.Y+g.vol.H, g.vol.X+g.vol.W, g.vol.Y+g.vol.H, 1, k.line.Alpha(0.6))
		if prev > 0 {
			for x := pr.X; x < pr.X+pr.W; x += 6 {
				p.Line(x, y(prev), x+3, y(prev), 1, k.muted)
			}
		}
		line := make([][2]float32, n)
		avg := make([][2]float32, 0, n)
		var area ui.Path
		bw := max(1, pr.W/span*0.7)
		for i, pt := range pts {
			x := xAt(i)
			line[i] = [2]float32{x, y(pt.Price)}
			if pt.Avg > 0 {
				avg = append(avg, [2]float32{x, y(pt.Avg)})
			}
			if i == 0 {
				area.MoveTo(x, pr.Y+pr.H)
			}
			area.LineTo(x, y(pt.Price))
			if vmax > 0 {
				up := pt.Price
				if i > 0 {
					up -= pts[i-1].Price
				} else if prev > 0 {
					up -= prev
				}
				vh := g.vol.H * float32(pt.Volume) / float32(vmax)
				p.Fill(ui.Rect{X: x - bw/2, Y: g.vol.Y + g.vol.H - vh, W: bw, H: max(0.5, vh)}, k.tone(up).Alpha(0.55), 0)
			}
		}
		area.LineTo(line[n-1][0], pr.Y+pr.H).Close()
		p.FillPath(&area, col.Alpha(0.10))
		if len(avg) > 1 {
			p.StrokePolyline(avg, 1, k.gold)
		}
		p.StrokePolyline(line, 1.6, col)
		p.Text(pr.X+2, pr.Y+1, price(hi), 10, k.faint)
		p.Text(pr.X+2, pr.Y+pr.H-12, price(lo), 10, k.faint)
		if prev > 0 {
			s := "昨收 " + price(prev)
			p.Text(pr.X+pr.W-textW(s, 10)-2, pr.Y+1, s, 10, k.faint)
		}
		base := r.Y + r.H - 15
		hm := func(t time.Time) string { return t.In(feed.Shanghai).Format("15:04") }
		p.Text(pr.X, base, hm(first), 10, k.faint)
		p.Text(pr.X+pr.W-textW(hm(end), 10), base, hm(end), 10, k.faint)
		if cr.bar < 0 {
			return
		}
		x := xAt(cr.bar)
		v := pts[cr.bar].Price
		yy := y(v)
		if cr.y >= 0 && cr.y >= pr.Y-r.Y && cr.y <= pr.Y-r.Y+pr.H {
			yy = r.Y + cr.y
			v = hi - float64((yy-pr.Y)/pr.H)*(hi-lo)
		}
		crossLines(p, k, r, g, x, yy, price(v), hm(pts[cr.bar].Time))
	})
}

// crossLines draws the crosshair at x, y with its labels: the price on
// the right edge, the time under it.
func crossLines(p *ui.Painter, k ink, r ui.Rect, g chartGeom, x, y float32, priceLabel, timeLabel string) {
	lc := k.muted.Alpha(0.8)
	for yy := g.price.Y; yy < g.vol.Y+g.vol.H; yy += 4 {
		p.Line(x, yy, x, min(yy+2, g.vol.Y+g.vol.H), 1, lc)
	}
	if y >= g.price.Y && y <= g.price.Y+g.price.H {
		for xx := g.price.X; xx < g.price.X+g.price.W; xx += 4 {
			p.Line(xx, y, min(xx+2, g.price.X+g.price.W), y, 1, lc)
		}
		w := textW(priceLabel, 10) + 10
		p.Fill(ui.Rect{X: g.price.X + g.price.W - w, Y: y - 8, W: w, H: 16}, k.text, 3)
		p.Text(g.price.X+g.price.W-w+5, y-6, priceLabel, 10, k.bg)
	}
	w := textW(timeLabel, 10) + 10
	lx := max(r.X+2, min(x-w/2, r.X+r.W-w-2))
	p.Fill(ui.Rect{X: lx, Y: r.Y + r.H - 17, W: w, H: 16}, k.text, 3)
	p.Text(lx+5, r.Y+r.H-15, timeLabel, 10, k.bg)
}

// textW is about how wide s is at size, in the UI font: digits and
// Latin half an em and a bit, CJK an em.
func textW(s string, size float32) float32 {
	w := float32(0)
	for _, r := range s {
		if r < 0x2000 {
			w += size * 0.6
		} else {
			w += size
		}
	}
	return w
}

// barDate is a bar's date as its period shows it.
func barDate(t time.Time, period int) string {
	t = t.In(feed.Shanghai)
	if period == 3 {
		return t.Format("2006-01")
	}
	return t.Format("2006-01-02")
}

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// chartInfo is the line over the chart: the crosshair's bar, or the
// latest; for the K-lines, the MAs.
func (a *app) chartInfo(c *ui.Context, k ink, sym string, prev float64) {
	type item struct {
		label, value string
		col          ui.Color
	}
	var head string
	var items []item
	tone := func(v float64) ui.Color { return k.tone(v) }
	if a.period == 0 {
		pts := a.intraday[sym]
		if len(pts) == 0 {
			return
		}
		i := len(pts) - 1
		if a.cross.bar >= 0 && a.cross.bar < len(pts) {
			i = a.cross.bar
		}
		pt := pts[i]
		head = pt.Time.In(feed.Shanghai).Format("01-02 15:04")
		chg := 0.0
		if prev > 0 {
			chg = (pt.Price - prev) / prev
		}
		items = []item{{"价", price(pt.Price), tone(pt.Price - prev)}, {"涨跌", pct(chg), tone(chg)}}
		if pt.Avg > 0 {
			items = append(items, item{"均价", price(pt.Avg), k.gold})
		}
		items = append(items, item{"量", big(float64(pt.Volume)), k.text})
		if pt.Turnover > 0 {
			items = append(items, item{"额", big(pt.Turnover), k.text})
		}
	} else {
		all := a.candles[candleKey(sym, a.period)]
		if len(all) == 0 {
			return
		}
		i := len(all) - 1
		if a.cross.bar >= 0 && a.cross.bar < len(all) {
			i = a.cross.bar
		}
		x := all[i]
		pc := x.Open
		if i > 0 {
			pc = all[i-1].Close
		}
		chg := 0.0
		if pc > 0 {
			chg = (x.Close - pc) / pc
		}
		t := x.Time.In(feed.Shanghai)
		head = barDate(x.Time, a.period)
		if a.period == 1 {
			head += " " + weekdays[t.Weekday()]
		}
		items = []item{
			{"开", price(x.Open), tone(x.Open - pc)}, {"高", price(x.High), tone(x.High - pc)},
			{"低", price(x.Low), tone(x.Low - pc)}, {"收", price(x.Close), tone(x.Close - pc)},
			{"涨跌", pct(chg), tone(chg)}, {"量", big(float64(x.Volume)), k.text},
		}
		if x.Turnover > 0 {
			items = append(items, item{"额", big(x.Turnover), k.text})
		}
	}
	ui.Column(c.Key("info")).Gap(3).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
			col := k.muted
			if a.cross.bar >= 0 {
				col = k.link
			}
			ui.Text(c, head).FontSize(11).FontWeight(600).TextColor(col).SingleLine()
			for _, it := range items {
				ui.Row(c.Key(it.label)).Gap(3).Children(func() {
					ui.Text(c, it.label).FontSize(11).TextColor(k.faint)
					ui.Text(c, it.value).FontSize(11).FontWeight(550).TextColor(it.col).SingleLine()
				})
			}
		})
		if a.period != 0 {
			all := a.candles[candleKey(sym, a.period)]
			i := len(all) - 1
			if a.cross.bar >= 0 && a.cross.bar < len(all) {
				i = a.cross.bar
			}
			ui.Row(c.Key("ma")).Gap(12).Children(func() {
				for j, n := range []int{5, 10, 20} {
					m := movingAvg(all, n)
					v := "--"
					if i >= 0 && !math.IsNaN(m[i]) {
						v = price(m[i])
					}
					ui.Text(c, fmt.Sprintf("MA%d %s", n, v)).FontSize(10).FontWeight(550).TextColor([]ui.Color{k.gold, k.ma10, k.ma20}[j])
				}
			})
		}
	})
}
