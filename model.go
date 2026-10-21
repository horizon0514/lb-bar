package main

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"lb-bar/alerts"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

type page int

const (
	pageMain page = iota
	pageSettings
)

// The tabs of the settings.
const (
	tabGeneral = iota
	tabAccount
	tabAlerts
	tabKeys
	tabAbout
)

var tabNames = []string{"通用", "账号", "提醒", "快捷键", "关于"}

// The chart periods of the detail pane.
var periods = []string{"分时", "日K", "周K", "月K"}

// maxSubscribed is how many symbols are pushed at most: the API's limit.
const maxSubscribed = 500

// flash is a row's tint after its price moved.
type flash struct {
	until time.Time
	up    bool
}

const flashFor = 700 * time.Millisecond

// app is the panel's state. It is read and changed on the main thread;
// work on goroutines (spawn) hands results back through post.
type app struct {
	cfg      config
	prov     feed.Provider
	dial     func(feed.Creds, feed.Options) (feed.Provider, error)
	creds    feed.Creds
	credFrom string // "keychain", "file", "env" or ""
	credErr  string // the keychain could not be read
	keys     credStore
	dataDir  string
	demo     bool // the data is made up: 示例数据
	alerts   *alerts.Book

	// Hooks from the shell (main.go), or tests.
	post           func(func())
	spawn          func(func())
	later          func(time.Duration, func()) // fn on the main thread after d
	open           func(url string)
	hide           func()
	quit           func()
	changed        func(config)
	setShort       func(string) error
	resize         func(expanded bool)
	setTranslucent func(bool)
	notify         func(id, title, body string)
	copy           func(string)
	trayChanged    func() // the menu bar's title or menu may have changed
	loginItem      func() bool
	setLoginItem   func(bool) error
	now            func() time.Time

	page page

	// Connection
	connecting bool
	connErr    string
	reconnects int
	live       bool // the pushes are on
	seq        int  // the current connection, to drop stale replies

	// Data
	groups     []feed.Group
	groupIdx   int // a group's index; len(groups) is 持仓
	secs       map[string]feed.Security
	quotes     map[string]*feed.Quote
	stats      map[string]feed.Stats
	spark      map[string][]float64
	sparkAsked map[string]time.Time
	flashes    map[string]flash
	subscribed map[string]bool
	markets    map[string]feed.Market
	dir        []feed.Watched // the API's US securities, to search
	dirAsked   bool
	loadedAt   time.Time

	// Search and the list
	query      string
	input      ui.Handle
	list       ui.ListState
	listFocus  ui.Handle
	listScroll ui.ScrollState
	chosen     int
	results    []feed.Watched
	resultsFor string
	searchSeq  int
	focusQuery bool
	caretToEnd bool
	focusList  bool
	composing  string
	listOn     bool // the list has the keyboard, as the frame found
	toast      string

	// Detail, on the right while expanded
	expanded    bool
	detailSym   string
	period      int
	intraday    map[string][]feed.Point
	candles     map[string][]feed.Candle // by symbol and period
	news        map[string][]feed.News
	detailErr   string
	detailBusy  int
	detailFocus ui.Handle
	focusDetail bool
	detailOn    bool
	newsChosen  int
	detailSeq   int
	panesW      float32

	// Positions
	positions []feed.Position
	balances  []feed.Balance
	posErr    string
	posBusy   bool
	posAt     time.Time

	// The menu bar
	tickerIdx int

	// Settings
	tab          int
	editKey      string
	editSecret   string
	editToken    string
	changingCred bool
	checking     bool
	settingsMsg  string
	settingsErr  bool
	editShortcut string
	editTicker   string
	editExpiry   string
	// The alert form
	alertSym    string
	alertKind   int
	alertValue  string
	alertRepeat bool
	alertMsg    string
	alertErr    bool
	focusAlert  bool

	cheat bool

	// Pushes wait here for the main thread, a batch at a time.
	pushMu  sync.Mutex
	pending map[string][]feed.Push
	queued  bool
}

func newApp(cfg config, creds feed.Creds, from string, dial func(feed.Creds, feed.Options) (feed.Provider, error)) *app {
	a := &app{cfg: cfg, dial: dial, creds: creds, credFrom: from, chosen: -1}
	a.secs, a.quotes, a.stats = map[string]feed.Security{}, map[string]*feed.Quote{}, map[string]feed.Stats{}
	a.spark, a.sparkAsked, a.flashes = map[string][]float64{}, map[string]time.Time{}, map[string]flash{}
	a.subscribed, a.markets = map[string]bool{}, map[string]feed.Market{}
	a.intraday, a.candles, a.news = map[string][]feed.Point{}, map[string][]feed.Candle{}, map[string][]feed.News{}
	a.keys = credStore{secrets: mygoSecrets{}, env: os.Getenv}
	a.alerts = alerts.Open("")
	a.post = func(fn func()) { fn() }
	a.spawn = func(fn func()) { go fn() }
	a.later = func(_ time.Duration, fn func()) { fn() }
	a.open = func(string) {}
	a.hide = func() {}
	a.quit = func() {}
	a.changed = func(config) {}
	a.setShort = func(string) error { return nil }
	a.resize = func(bool) {}
	a.setTranslucent = func(bool) {}
	a.notify = func(string, string, string) {}
	a.copy = func(string) {}
	a.trayChanged = func() {}
	a.loginItem = func() bool { return false }
	a.setLoginItem = func(bool) error { return nil }
	a.now = time.Now
	a.editShortcut = cfg.Shortcut
	if !creds.Complete() {
		a.page, a.tab = pageSettings, tabAccount
	}
	a.listFocus.Focus() // waits for the list to be built
	return a
}

func (a *app) configured() bool { return a.creds.Complete() || a.demo }

func (a *app) options() feed.Options {
	r := a.cfg.Region
	if r == "global" {
		r = ""
	}
	return feed.Options{Region: r, Overnight: a.cfg.Overnight}
}

// connect dials the API with the credentials, then loads everything.
func (a *app) connect() {
	if a.dial == nil || !a.configured() {
		return
	}
	a.disconnect()
	a.seq++
	seq, creds, opts, dial := a.seq, a.creds, a.options(), a.dial
	a.connecting, a.connErr = true, ""
	a.spawn(func() {
		p, err := dial(creds, opts)
		a.post(func() {
			if seq != a.seq {
				if p != nil {
					_ = p.Close()
				}
				return
			}
			a.connecting = false
			if err != nil {
				a.connErr = feed.Friendly(err).Error()
				a.trayChanged()
				return
			}
			a.use(p)
		})
	})
}

// use starts using a provider: pushes, then the data.
func (a *app) use(p feed.Provider) {
	a.prov = p
	seq := a.seq
	p.OnPush(func(ps feed.Push) { a.queuePush(seq, ps) })
	p.OnReconnect(func(ok bool) {
		a.post(func() {
			if seq == a.seq {
				a.reconnected(ok)
			}
		})
	})
	a.load()
}

func (a *app) disconnect() {
	if a.prov != nil {
		p := a.prov
		a.prov = nil
		a.spawn(func() { _ = p.Close() })
	}
	a.live = false
	a.subscribed = map[string]bool{}
}

// load loads the watchlist, then the names and quotes of everything
// shown, subscribes to their pushes, and the market status.
func (a *app) load() {
	p, seq := a.prov, a.seq
	if p == nil {
		return
	}
	a.connErr = ""
	a.spawn(func() {
		ctx := context.Background()
		gs, err := p.Groups(ctx)
		a.post(func() {
			if seq != a.seq {
				return
			}
			if err != nil {
				a.connErr = "读取自选股失败：" + err.Error()
				return
			}
			a.setGroups(gs)
			a.loadedAt = a.now()
			a.refreshQuotes(true)
		})
	})
	a.refreshMarkets()
	if len(a.cfg.Tickers) > 0 || len(a.alerts.Symbols()) > 0 {
		a.loadPositions(false)
	}
}

// setGroups keeps the watchlist groups that have securities, and stays
// on the group shown.
func (a *app) setGroups(gs []feed.Group) {
	var keep []feed.Group
	for _, g := range gs {
		if len(g.Securities) > 0 {
			keep = append(keep, g)
		}
	}
	a.groups = keep
	a.groupIdx = 0
	for i, g := range keep {
		if g.ID == a.cfg.Group {
			a.groupIdx = i
		}
	}
	for _, g := range keep {
		for _, w := range g.Securities {
			if _, ok := a.secs[w.Symbol]; !ok && w.Name != "" {
				a.secs[w.Symbol] = feed.Security{Symbol: w.Symbol, Name: w.Name}
			}
		}
	}
	if a.chosen >= len(a.rows()) {
		a.chosen = len(a.rows()) - 1
	}
	if a.chosen < 0 && len(a.rows()) > 0 {
		a.chosen = 0
	}
	a.trayChanged()
}

// wanted are the symbols whose quotes the app keeps live, most needed
// first: the menu bar's, the alerts', the group shown, the holdings, the
// detail's, the other groups.
func (a *app) wanted() []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] && len(out) < maxSubscribed {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range a.tickers() {
		add(s)
	}
	for _, s := range a.alerts.Symbols() {
		add(s)
	}
	for _, w := range a.rows() {
		add(w.Symbol)
	}
	for _, p := range a.positions {
		add(p.Symbol)
	}
	add(a.detailSym)
	for _, g := range a.groups {
		for _, w := range g.Securities {
			add(w.Symbol)
		}
	}
	return out
}

// refreshQuotes fetches the quotes of what is wanted, in one call per
// 500, and the names not known yet; with sub, subscribes to the pushes
// of those not subscribed.
func (a *app) refreshQuotes(sub bool) {
	p, seq := a.prov, a.seq
	if p == nil {
		return
	}
	syms := a.wanted()
	if len(syms) == 0 {
		return
	}
	var unnamed, unsub []string
	for _, s := range syms {
		if sec, ok := a.secs[s]; !ok || sec.Currency == "" {
			unnamed = append(unnamed, s)
		}
		if sub && !a.subscribed[s] {
			unsub = append(unsub, s)
		}
	}
	a.spawn(func() {
		ctx := context.Background()
		var secs []feed.Security
		var serr error
		if len(unnamed) > 0 {
			secs, serr = p.Static(ctx, unnamed)
		}
		qs, err := p.Quotes(ctx, syms)
		var suberr error
		if len(unsub) > 0 {
			suberr = p.Subscribe(ctx, unsub)
		}
		a.post(func() {
			if seq != a.seq {
				return
			}
			if serr == nil {
				for _, s := range secs {
					a.secs[s.Symbol] = s
				}
			}
			if err != nil {
				a.connErr = "读取行情失败：" + err.Error()
			} else {
				for _, q := range qs {
					q := q
					a.setQuote(&q)
				}
			}
			if len(unsub) > 0 {
				if suberr != nil {
					a.connErr = "订阅推送失败：" + suberr.Error()
				} else {
					for _, s := range unsub {
						a.subscribed[s] = true
					}
					a.live = true
				}
			}
			a.trayChanged()
		})
	})
}

// setQuote keeps a quote, its extended sessions from the last one where
// it has none, and checks the alerts.
func (a *app) setQuote(q *feed.Quote) {
	if old := a.quotes[q.Symbol]; old != nil {
		if q.Pre == nil {
			q.Pre = old.Pre
		}
		if q.Post == nil {
			q.Post = old.Post
		}
	}
	a.quotes[q.Symbol] = q
	a.checkAlerts(*q)
}

// queuePush keeps a push for the next batch, on the push's goroutine.
// Pushes come many a second; the main thread takes them in batches.
func (a *app) queuePush(seq int, p feed.Push) {
	a.pushMu.Lock()
	if a.pending == nil {
		a.pending = map[string][]feed.Push{}
	}
	a.pending[p.Symbol] = append(a.pending[p.Symbol], p)
	queue := !a.queued
	a.queued = true
	a.pushMu.Unlock()
	if queue {
		a.later(150*time.Millisecond, func() {
			if seq == a.seq {
				a.flushPushes()
			}
		})
	}
}

// flushPushes applies the pushes waiting: the quotes, their flashes, the
// sparklines' last minute, and the alerts.
func (a *app) flushPushes() {
	a.pushMu.Lock()
	pending := a.pending
	a.pending, a.queued = nil, false
	a.pushMu.Unlock()
	now := a.now()
	for sym, ps := range pending {
		q := a.quotes[sym]
		if q == nil {
			q = &feed.Quote{Symbol: sym}
			a.quotes[sym] = q
		}
		before := q.Last
		for _, p := range ps {
			q.Apply(p)
			if p.Session == feed.Regular {
				a.sparkPush(sym, p)
			}
		}
		if before != 0 && q.Last != before {
			a.flashes[sym] = flash{until: now.Add(flashFor), up: q.Last > before}
		}
		a.checkAlerts(*q)
	}
	if len(pending) > 0 {
		a.live = true
		a.trayChanged()
	}
}

// sparkPush puts a pushed price on the day's line: a new minute adds a
// point, the same minute moves the last.
func (a *app) sparkPush(sym string, p feed.Push) {
	pts := a.intraday[sym]
	if len(pts) == 0 {
		return
	}
	last := &pts[len(pts)-1]
	m := p.Time.Truncate(time.Minute)
	if !p.Time.IsZero() && m.After(last.Time) {
		pts = append(pts, feed.Point{Time: m, Price: p.Last, Avg: last.Avg})
	} else {
		last.Price = p.Last
	}
	a.intraday[sym] = pts
	a.spark[sym] = prices(pts)
}

func prices(pts []feed.Point) []float64 {
	out := make([]float64, len(pts))
	for i, p := range pts {
		out[i] = p.Price
	}
	return out
}

// reconnected: the SDK reconnected and, ok, resubscribed. The quotes
// missed meanwhile are fetched; unsubscribed, they are subscribed again.
func (a *app) reconnected(ok bool) {
	a.reconnects++
	if !ok {
		a.subscribed = map[string]bool{}
	}
	a.refreshQuotes(true)
}

// sparkFor is the day's prices of sym, fetched once in a while: none
// until they come.
func (a *app) sparkFor(sym string) []float64 {
	if a.prov == nil {
		return a.spark[sym]
	}
	at, asked := a.sparkAsked[sym]
	if !asked || a.now().Sub(at) > 10*time.Minute {
		a.sparkAsked[sym] = a.now()
		a.fetchIntraday(sym)
	}
	return a.spark[sym]
}

func (a *app) fetchIntraday(sym string) {
	p, seq := a.prov, a.seq
	a.spawn(func() {
		pts, err := p.Intraday(context.Background(), sym)
		a.post(func() {
			if seq != a.seq || err != nil {
				return
			}
			a.intraday[sym] = pts
			a.spark[sym] = prices(pts)
		})
	})
}

// refreshMarkets asks the markets' status; failing, the timetable stands in.
func (a *app) refreshMarkets() {
	p, seq := a.prov, a.seq
	if p == nil {
		return
	}
	a.spawn(func() {
		ms, err := p.Markets(context.Background())
		a.post(func() {
			if seq != a.seq || err != nil {
				return
			}
			for _, m := range ms {
				a.markets[m.Market] = m
			}
			a.trayChanged()
		})
	})
}

// phase is a market's phase: the API's, or the timetable's.
func (a *app) phase(market string) feed.Phase {
	if m, ok := a.markets[market]; ok && a.now().Sub(m.At) < 3*time.Minute {
		// The API's status knows no overnight session: closed then is
		// the timetable's night.
		if m.Phase == feed.Closed && market == "US" && feed.ScheduledPhase(market, a.now()) == feed.Night {
			return feed.Night
		}
		return m.Phase
	}
	return feed.ScheduledPhase(market, a.now())
}

// tick runs every minute: the market status, the holdings when shown,
// the expiry reminder, and the quotes now and then (pushes can be missed).
func (a *app) tick() {
	if a.prov == nil {
		return
	}
	a.refreshMarkets()
	if a.showingPositions() && a.now().Sub(a.posAt) > time.Minute {
		a.loadPositions(false)
	}
	if a.now().Sub(a.loadedAt) > 5*time.Minute {
		a.loadedAt = a.now()
		a.refreshQuotes(true)
	}
	a.remindExpiry()
}

// rows are the securities the list shows: the search's results, the
// group's, or the holdings.
func (a *app) rows() []feed.Watched {
	if a.searching() {
		return a.results
	}
	if a.showingPositions() {
		out := make([]feed.Watched, len(a.positions))
		for i, p := range a.positions {
			out[i] = feed.Watched{Symbol: p.Symbol, Name: p.Name}
		}
		return out
	}
	if a.groupIdx < len(a.groups) {
		return a.groups[a.groupIdx].Securities
	}
	return nil
}

func (a *app) searching() bool { return strings.TrimSpace(a.query) != "" }

func (a *app) showingPositions() bool {
	return !a.searching() && a.groupIdx >= len(a.groups) && a.configured()
}

// chosenSym is the symbol of the chosen row.
func (a *app) chosenSym() string {
	rs := a.rows()
	if a.chosen < 0 || a.chosen >= len(rs) {
		return ""
	}
	return rs[a.chosen].Symbol
}

// name is a security's name, as known.
func (a *app) name(sym string) string {
	if s, ok := a.secs[sym]; ok && s.Name != "" {
		return s.Name
	}
	for _, w := range a.rows() {
		if w.Symbol == sym && w.Name != "" {
			return w.Name
		}
	}
	return feed.Code(sym)
}

// choose chooses row i; expanded, the detail follows, a moment later.
func (a *app) choose(i int) {
	n := len(a.rows())
	if n == 0 {
		a.chosen = -1
		return
	}
	a.chosen = max(0, min(i, n-1))
	a.list.ScrollIntoView(a.chosen)
	if a.expanded {
		sym := a.chosenSym()
		a.detailSeq++
		seq := a.detailSeq
		a.later(250*time.Millisecond, func() {
			if seq == a.detailSeq {
				a.showDetail(sym)
			}
		})
	}
}

// switchGroup moves to the group (or 持仓) by, or to i with at.
func (a *app) setGroup(i int) {
	n := len(a.groups) + 1 // and 持仓
	i = ((i % n) + n) % n
	a.groupIdx = i
	a.query, a.results = "", nil
	if i < len(a.groups) {
		a.cfg.Group = a.groups[i].ID
		a.changed(a.cfg)
	} else {
		a.loadPositions(false)
	}
	a.chosen = -1
	a.choose(0)
	a.listScroll.Y = 0
	a.refreshQuotes(true)
	a.focusList = true
}

// expand opens the detail of the chosen row on the right, or closes it.
func (a *app) expand(on bool) {
	if on == a.expanded {
		if on {
			a.showDetail(a.chosenSym())
		}
		return
	}
	a.expanded = on
	a.resize(on)
	if on {
		a.showDetail(a.chosenSym())
	} else {
		a.focusList = true
	}
}

// showDetail loads what the detail pane shows of sym: its day, its stats
// and news, and the K-line of the period shown.
func (a *app) showDetail(sym string) {
	if sym == "" {
		return
	}
	changed := sym != a.detailSym
	a.detailSym, a.detailErr = sym, ""
	if changed {
		a.newsChosen = -1
	}
	p, seq := a.prov, a.seq
	if p == nil {
		return
	}
	if !a.subscribed[sym] {
		a.refreshQuotes(true)
	}
	if _, ok := a.intraday[sym]; !ok || changed {
		a.sparkAsked[sym] = a.now()
		a.fetchIntraday(sym)
	}
	if _, ok := a.stats[sym]; !ok {
		a.spawn(func() {
			st, err := p.Stats(context.Background(), []string{sym})
			a.post(func() {
				if seq == a.seq && err == nil {
					for _, s := range st {
						a.stats[s.Symbol] = s
					}
				}
			})
		})
	}
	if _, ok := a.news[sym]; !ok {
		a.detailBusy++
		a.spawn(func() {
			ns, err := p.News(context.Background(), sym)
			a.post(func() {
				a.detailBusy--
				if seq != a.seq {
					return
				}
				if err != nil {
					ns = nil
				}
				a.news[sym] = ns
			})
		})
	}
	a.loadCandles()
}

func candleKey(sym string, period int) string { return sym + "|" + strconv.Itoa(period) }

// loadCandles fetches the K-line of the period shown, once.
func (a *app) loadCandles() {
	if a.period == 0 || a.detailSym == "" || a.prov == nil {
		return
	}
	key := candleKey(a.detailSym, a.period)
	if _, ok := a.candles[key]; ok {
		return
	}
	a.candles[key] = nil
	p, seq, sym, per := a.prov, a.seq, a.detailSym, feed.Period(a.period-1)
	a.detailBusy++
	a.spawn(func() {
		cs, err := p.Candles(context.Background(), sym, per, 90)
		a.post(func() {
			a.detailBusy--
			if seq != a.seq {
				return
			}
			if err != nil {
				delete(a.candles, key)
				a.detailErr = "K 线读取失败：" + err.Error()
				return
			}
			a.candles[key] = cs
		})
	})
}

func (a *app) setPeriod(i int) {
	a.period = (i + len(periods)) % len(periods)
	a.loadCandles()
}

// loadPositions reads the holdings and balances: read only. With
// force it asks again even if just read.
func (a *app) loadPositions(force bool) {
	p, seq := a.prov, a.seq
	if p == nil || a.posBusy || (!force && !a.posAt.IsZero() && a.now().Sub(a.posAt) < 30*time.Second) {
		return
	}
	a.posBusy = true
	a.spawn(func() {
		ctx := context.Background()
		ps, err := p.Positions(ctx)
		var bs []feed.Balance
		if err == nil {
			bs, err = p.Balances(ctx)
		}
		a.post(func() {
			a.posBusy = false
			if seq != a.seq {
				return
			}
			a.posAt = a.now()
			if err != nil {
				a.posErr = err.Error()
				return
			}
			a.posErr = ""
			a.positions, a.balances = ps, bs
			for _, p := range ps {
				if _, ok := a.secs[p.Symbol]; !ok && p.Name != "" {
					a.secs[p.Symbol] = feed.Security{Symbol: p.Symbol, Name: p.Name, Currency: p.Currency}
				}
			}
			a.refreshQuotes(true)
		})
	})
}

// pnl is a holding's market value and profit and loss today and in all,
// in its currency; ok is false without a quote.
func (a *app) pnl(p feed.Position) (value, today, total float64, ok bool) {
	q := a.quotes[p.Symbol]
	if q == nil || q.Last == 0 {
		return 0, 0, 0, false
	}
	value = p.Quantity * q.Last
	if q.PrevClose > 0 {
		today = p.Quantity * (q.Last - q.PrevClose)
	}
	if p.Cost > 0 {
		total = p.Quantity * (q.Last - p.Cost)
	}
	return value, today, total, true
}

// Search

// runSearch searches as the query changes: what the app knows at once,
// then a code checked with the API, a moment later.
func (a *app) runSearch() {
	q := strings.TrimSpace(a.query)
	a.searchSeq++
	if q == "" {
		a.results, a.resultsFor = nil, ""
		a.chosen = -1
		a.choose(0)
		return
	}
	a.loadDirectory()
	a.results = searchKnown(q, a.known(), 20)
	a.resultsFor = q
	a.chosen = -1
	a.choose(0)
	seq := a.searchSeq
	cands := codeCandidates(q)
	var ask []string
	for _, c := range cands {
		if s, ok := a.secs[c]; !ok || s.Currency == "" {
			ask = append(ask, c)
		}
	}
	syms := make([]string, 0, len(a.results)+len(cands))
	for _, r := range a.results {
		syms = append(syms, r.Symbol)
	}
	a.later(250*time.Millisecond, func() {
		if seq != a.searchSeq || a.prov == nil {
			return
		}
		p := a.prov
		a.spawn(func() {
			ctx := context.Background()
			var secs []feed.Security
			if len(ask) > 0 {
				secs, _ = p.Static(ctx, ask)
			}
			all := slices.Clone(syms)
			for _, s := range secs {
				if !slices.Contains(all, s.Symbol) {
					all = append(all, s.Symbol)
				}
			}
			var qs []feed.Quote
			if len(all) > 0 {
				qs, _ = p.Quotes(ctx, all)
			}
			a.post(func() {
				for _, s := range secs {
					a.secs[s.Symbol] = s
				}
				for _, q := range qs {
					q := q
					if a.quotes[q.Symbol] == nil || !a.subscribed[q.Symbol] {
						a.setQuote(&q)
					}
				}
				if seq != a.searchSeq {
					return
				}
				// Codes the API knows go first.
				var front []feed.Watched
				for _, c := range cands {
					if s, ok := a.secs[c]; ok && s.Currency != "" {
						front = append(front, feed.Watched{Symbol: c, Name: s.Name})
					}
				}
				rest := a.results[:0:0]
				for _, r := range a.results {
					if !slices.ContainsFunc(front, func(w feed.Watched) bool { return w.Symbol == r.Symbol }) {
						rest = append(rest, r)
					}
				}
				sym := a.chosenSym()
				a.results = append(front, rest...)
				if i := slices.IndexFunc(a.results, func(w feed.Watched) bool { return w.Symbol == sym }); i >= 0 && a.chosen > 0 {
					a.chosen = i
				} else {
					a.chosen = -1
					a.choose(0)
				}
			})
		})
	})
}

// known are the securities to search in.
func (a *app) known() []feed.Watched {
	var out []feed.Watched
	for _, g := range a.groups {
		out = append(out, g.Securities...)
	}
	for _, p := range a.positions {
		out = append(out, feed.Watched{Symbol: p.Symbol, Name: p.Name})
	}
	for _, s := range a.secs {
		n := s.Name
		if s.NameEn != "" && s.NameEn != n {
			n += "\x00" + s.NameEn
		}
		out = append(out, feed.Watched{Symbol: s.Symbol, Name: n})
	}
	return append(out, a.dir...)
}

// loadDirectory fetches the API's list of US securities, once a run.
func (a *app) loadDirectory() {
	if a.dirAsked || a.prov == nil {
		return
	}
	a.dirAsked = true
	p := a.prov
	a.spawn(func() {
		ws, err := p.Directory(context.Background())
		a.post(func() {
			if err != nil {
				a.dirAsked = false
				return
			}
			a.dir = ws
			if a.searching() {
				a.runSearch()
			}
		})
	})
}

// Alerts

func (a *app) checkAlerts(q feed.Quote) {
	for _, f := range a.alerts.Check(q, a.now()) {
		_, rate := f.Quote.Change()
		name := a.name(q.Symbol)
		title := name + " " + feed.Code(q.Symbol) + " " + f.Rule.Describe()
		body := "现价 " + price(q.Last) + "  " + pct(rate) + " · 北京时间 " + f.At.In(feed.Shanghai).Format("15:04")
		if a.cfg.Notify {
			a.notify("alert-"+f.Rule.ID, title, body)
		}
		a.toast = "🔔 " + title
	}
}

// addAlert adds the rule of the form.
func (a *app) addAlert() {
	sym := strings.TrimSpace(a.alertSym)
	if cs := codeCandidates(sym); len(cs) == 1 {
		sym = cs[0]
	} else if hits := searchKnown(sym, a.known(), 1); len(hits) == 1 {
		sym = hits[0].Symbol
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(a.alertValue), "%"), 64)
	if err != nil {
		a.alertMsg, a.alertErr = "数值填数字，例如 500 或 3", true
		return
	}
	r, err := a.alerts.Add(alerts.Rule{Symbol: sym, Name: a.name(sym), Kind: alerts.Kinds[a.alertKind], Value: v, Repeat: a.alertRepeat})
	if err != nil {
		a.alertMsg, a.alertErr = err.Error(), true
		return
	}
	a.alertMsg, a.alertErr = "已添加："+r.Name+" "+r.Describe(), false
	if q := a.quotes[sym]; q != nil && r.Met(*q) {
		a.alertMsg += "（现价已满足，马上会提醒一次）"
	}
	a.alertValue = ""
	a.refreshQuotes(true)
	if q := a.quotes[sym]; q != nil {
		a.checkAlerts(*q)
	}
}

// newAlert opens the alert form for the chosen security.
func (a *app) newAlert() {
	if s := a.chosenSym(); s != "" {
		a.alertSym = s
		if q := a.quotes[s]; q != nil && q.Last > 0 {
			a.alertValue = price(q.Last)
		}
	}
	a.alertMsg = ""
	a.openTab(tabAlerts)
	a.focusAlert = true
}

// The menu bar

// tickers are the symbols the menu bar shows in turn.
func (a *app) tickers() []string {
	if len(a.cfg.Tickers) > 0 {
		return a.cfg.Tickers
	}
	for _, g := range a.groups {
		if len(g.Securities) > 0 {
			return []string{g.Securities[0].Symbol}
		}
	}
	return nil
}

// togglePin puts the chosen security in the menu bar, or takes it off.
func (a *app) togglePin() {
	sym := a.chosenSym()
	if sym == "" {
		return
	}
	if i := slices.Index(a.cfg.Tickers, sym); i >= 0 {
		a.cfg.Tickers = slices.Delete(slices.Clone(a.cfg.Tickers), i, i+1)
		a.toast = "已从菜单栏移除 " + a.name(sym)
	} else {
		a.cfg.Tickers = append(slices.Clone(a.cfg.Tickers), sym)
		a.toast = "已固定到菜单栏：" + a.name(sym)
		a.refreshQuotes(true)
	}
	a.changed(a.cfg)
	a.trayChanged()
}

// rotate shows the next ticker, every RotateSec.
func (a *app) rotate() {
	if a.cfg.Rotate {
		a.tickerIdx++
		a.trayChanged()
	}
}

// trayTitle is the menu bar's text: 腾讯控股 512.50 +1.20%, or nothing
// (the icon alone).
func (a *app) trayTitle() string {
	if a.cfg.Menubar == "icon" {
		return ""
	}
	ts := a.tickers()
	if len(ts) == 0 {
		return ""
	}
	sym := ts[a.tickerIdx%len(ts)]
	if !a.cfg.Rotate {
		sym = ts[0]
	}
	label := shortName(a.name(sym), sym)
	if a.cfg.MenubarCode {
		label = feed.Code(sym)
	}
	q := a.quotes[sym]
	if q == nil || q.Last == 0 {
		return label + " --"
	}
	_, rate := q.Change()
	s := label + " " + price(q.Last) + " " + pct(rate)
	if a.cfg.Dot {
		s = a.dot(direction(rate)) + s
	}
	return s
}

// dot is the menu bar's colored dot for a direction, by the color
// convention: macOS draws the menu bar's text in one color.
func (a *app) dot(dir int) string {
	switch {
	case dir == 0:
		return "⚪ "
	case (dir > 0) != a.cfg.GreenUp:
		return "🔴 "
	}
	return "🟢 "
}

// trayLine is the tray menu's first line: the markets' status.
func (a *app) trayLine() string {
	var parts []string
	for _, m := range []string{"HK", "US", "CN"} {
		parts = append(parts, feed.MarketName(m)+" "+a.phase(m).Label(m))
	}
	return strings.Join(parts, " · ")
}

// The access token

// expiry is when the access token expires, as known.
func (a *app) expiry() (time.Time, bool) {
	if t, ok := tokenExpiry(a.creds.AccessToken); ok {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, a.cfg.TokenExpires); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// daysLeft is how many days the token has left (negative: expired).
func (a *app) daysLeft() (int, bool) {
	t, ok := a.expiry()
	if !ok || a.demo {
		return 0, false
	}
	d := t.Sub(a.now())
	if d < 0 {
		return -1, true
	}
	return int(d.Hours() / 24), true
}

const remindDays = 14

// expiryWarning is what the panel says when the token expires soon.
func (a *app) expiryWarning() string {
	n, ok := a.daysLeft()
	if !ok || n >= remindDays {
		return ""
	}
	t, _ := a.expiry()
	if n < 0 {
		return "Access Token 已过期，到长桥开发者中心重新生成后在设置里更换"
	}
	return "Access Token 还有 " + strconv.Itoa(n) + " 天过期（" + t.In(feed.Shanghai).Format("2006-01-02") + "），请到长桥开发者中心续期"
}

// remindExpiry notifies once a day in the last week of the token.
func (a *app) remindExpiry() {
	n, ok := a.daysLeft()
	today := a.now().In(feed.Shanghai).Format("2006-01-02")
	if !ok || n >= 7 || a.cfg.Reminded == today {
		return
	}
	a.cfg.Reminded = today
	a.changed(a.cfg)
	body := "还有 " + strconv.Itoa(max(n, 0)) + " 天，到期后行情将无法更新。打开 open.longbridge.com 重新生成，再到 LB Bar 设置 › 账号 里更换。"
	if n < 0 {
		body = "已过期，行情无法更新。打开 open.longbridge.com 重新生成，再到 LB Bar 设置 › 账号 里更换。"
	}
	a.notify("token-expiry", "长桥 Access Token 即将过期", body)
}

// Navigation

func (a *app) openSettings() { a.openTab(a.tab) }

func (a *app) openTab(i int) {
	a.page, a.cheat = pageSettings, false
	a.tab = i
	a.settingsMsg = ""
}

func (a *app) back() {
	if !a.configured() {
		a.hide()
		return
	}
	a.page = pageMain
	a.focusList = true
}

func (a *app) openWeb(sym string) {
	if sym != "" {
		a.open(quoteURL(sym))
	}
}

// shown readies the panel as it shows: typing goes to the search, the
// holdings and the quotes refreshed if old.
func (a *app) shown() {
	a.cheat = false
	if a.page == pageMain {
		a.focusQuery, a.caretToEnd = true, true
		a.input.Focus()
	}
	if a.showingPositions() && a.now().Sub(a.posAt) > 30*time.Second {
		a.loadPositions(false)
	}
	if a.prov != nil && a.now().Sub(a.loadedAt) > 2*time.Minute {
		a.loadedAt = a.now()
		a.refreshQuotes(false)
	}
}

// showPositions shows the holdings.
func (a *app) showPositions() {
	if !a.configured() {
		a.openTab(tabAccount)
		return
	}
	a.page, a.query, a.results = pageMain, "", nil
	a.setGroup(len(a.groups))
}
