package feed

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"time"
)

// Mock is a Provider of made-up example data, for tests, screenshots
// and the demo mode (LBBAR_DEMO=1). Nothing in it is real: the app labels
// it 示例数据.
type Mock struct {
	Now   func() time.Time
	Fail  error // when set, every call fails with it
	Delay time.Duration

	mu         sync.Mutex
	quotes     map[string]*Quote
	push       func(Push)
	recon      func(bool)
	subscribed map[string]bool
	Calls      map[string]int // by method, for tests
	closed     bool
}

type mockSec struct {
	Security
	last, prev float64
	cap, pe    float64
}

var mockSecs = []mockSec{
	{Security{Symbol: "700.HK", Name: "腾讯控股", NameEn: "TENCENT", Currency: "HKD", LotSize: 100, TotalShares: 9_092_644_829, EPS: 29.82}, 512.5, 506.4, 4.66e12, 17.2},
	{Security{Symbol: "9988.HK", Name: "阿里巴巴-W", NameEn: "BABA-W", Currency: "HKD", LotSize: 100, TotalShares: 19_901_959_514, EPS: 4.24}, 128.3, 131.1, 2.55e12, 15.1},
	{Security{Symbol: "3690.HK", Name: "美团-W", NameEn: "MEITUAN-W", Currency: "HKD", LotSize: 100, TotalShares: 6_100_000_000, EPS: 6.1}, 98.75, 97.2, 6.02e11, 16.2},
	{Security{Symbol: "1810.HK", Name: "小米集团-W", NameEn: "XIAOMI-W", Currency: "HKD", LotSize: 200, TotalShares: 25_900_000_000, EPS: 1.42}, 41.85, 41.85, 1.08e12, 29.5},
	{Security{Symbol: "AAPL.US", Name: "苹果", NameEn: "Apple", Currency: "USD", LotSize: 1, TotalShares: 14_594_180_000, EPS: 8.83}, 242.18, 238.6, 3.53e12, 27.4},
	{Security{Symbol: "NVDA.US", Name: "英伟达", NameEn: "NVIDIA", Currency: "USD", LotSize: 1, TotalShares: 24_300_000_000, EPS: 4.1}, 186.4, 190.02, 4.53e12, 45.5},
	{Security{Symbol: "TSLA.US", Name: "特斯拉", NameEn: "Tesla", Currency: "USD", LotSize: 1, TotalShares: 3_949_547_394, EPS: 0.96}, 418.9, 402.3, 1.65e12, 0},
	{Security{Symbol: "MSFT.US", Name: "微软", NameEn: "Microsoft", Currency: "USD", LotSize: 1, TotalShares: 7_430_000_000, EPS: 13.6}, 517.03, 514.6, 3.84e12, 38.0},
	{Security{Symbol: "BABA.US", Name: "阿里巴巴", NameEn: "Alibaba", Currency: "USD", LotSize: 1, TotalShares: 2_399_050_494, EPS: 4.4}, 132.6, 135.0, 3.18e11, 15.0},
	{Security{Symbol: "600519.SH", Name: "贵州茅台", NameEn: "Moutai", Currency: "CNY", LotSize: 100, TotalShares: 1_250_081_601, EPS: 65.14}, 1452.0, 1440.5, 1.82e12, 22.3},
	{Security{Symbol: "300750.SZ", Name: "宁德时代", NameEn: "CATL", Currency: "CNY", LotSize: 100, TotalShares: 4_400_000_000, EPS: 11.5}, 268.66, 272.1, 1.18e12, 23.4},
	{Security{Symbol: "000001.SZ", Name: "平安银行", NameEn: "PAB", Currency: "CNY", LotSize: 100, TotalShares: 19_405_918_198, EPS: 2.24}, 11.42, 11.36, 2.22e11, 5.1},
}

func mockSecOf(sym string) (mockSec, bool) {
	sym = strings.TrimLeft(strings.ToUpper(sym), "0")
	for _, s := range mockSecs {
		if strings.TrimLeft(s.Symbol, "0") == sym {
			return s, true
		}
	}
	return mockSec{}, false
}

func (m *Mock) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Mock) begin(name string) error {
	m.mu.Lock()
	if m.Calls == nil {
		m.Calls = map[string]int{}
	}
	m.Calls[name]++
	fail, d := m.Fail, m.Delay
	m.mu.Unlock()
	if d > 0 {
		time.Sleep(d)
	}
	return fail
}

// Count is how many times method was called.
func (m *Mock) Count(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Calls[method]
}

func (m *Mock) Groups(context.Context) ([]Group, error) {
	if err := m.begin("Groups"); err != nil {
		return nil, err
	}
	var all, hk, us, cn Group
	all = Group{ID: 1, Name: "全部"}
	hk, us, cn = Group{ID: 2, Name: "港股"}, Group{ID: 3, Name: "美股"}, Group{ID: 4, Name: "A股"}
	for _, s := range mockSecs {
		w := Watched{Symbol: s.Symbol, Name: s.Name}
		all.Securities = append(all.Securities, w)
		switch MarketOf(s.Symbol) {
		case "HK":
			hk.Securities = append(hk.Securities, w)
		case "US":
			us.Securities = append(us.Securities, w)
		default:
			cn.Securities = append(cn.Securities, w)
		}
	}
	return []Group{all, hk, us, cn, {ID: -6, Name: "holdings"}}, nil
}

func (m *Mock) Static(_ context.Context, syms []string) ([]Security, error) {
	if err := m.begin("Static"); err != nil {
		return nil, err
	}
	var out []Security
	for _, sym := range syms {
		if s, ok := mockSecOf(sym); ok {
			out = append(out, s.Security)
		}
	}
	return out, nil
}

func (m *Mock) quote(s mockSec) *Quote {
	if m.quotes == nil {
		m.quotes = map[string]*Quote{}
	}
	if q := m.quotes[s.Symbol]; q != nil {
		return q
	}
	q := &Quote{Symbol: s.Symbol, Last: s.last, PrevClose: s.prev, Open: s.prev * 1.003, High: max(s.last, s.prev) * 1.012,
		Low: min(s.last, s.prev) * 0.991, Volume: int64(s.cap / s.last / 400), Turnover: s.cap / 400, Time: m.now()}
	if MarketOf(s.Symbol) == "US" {
		q.Post = &Ext{Last: s.last * 1.004, PrevClose: s.last, High: s.last * 1.006, Low: s.last * 0.998, Volume: 812_300, Time: m.now()}
	}
	m.quotes[s.Symbol] = q
	return q
}

func (m *Mock) Quotes(_ context.Context, syms []string) ([]Quote, error) {
	if err := m.begin("Quotes"); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Quote
	for _, sym := range syms {
		if s, ok := mockSecOf(sym); ok {
			q := *m.quote(s)
			if q.Pre != nil {
				e := *q.Pre
				q.Pre = &e
			}
			if q.Post != nil {
				e := *q.Post
				q.Post = &e
			}
			out = append(out, q)
		}
	}
	return out, nil
}

func (m *Mock) Stats(_ context.Context, syms []string) ([]Stats, error) {
	if err := m.begin("Stats"); err != nil {
		return nil, err
	}
	var out []Stats
	for _, sym := range syms {
		if s, ok := mockSecOf(sym); ok {
			out = append(out, Stats{Symbol: s.Symbol, MarketCap: s.cap, PE: s.pe, PB: 3.1, TurnoverRate: 0.0042})
		}
	}
	return out, nil
}

// wave is a made-up but steady price path from prev to last, i of n.
func wave(prev, last float64, i, n int, seed float64) float64 {
	t := float64(i) / float64(max(1, n-1))
	base := prev + (last-prev)*t
	amp := math.Abs(last-prev)*0.6 + prev*0.004
	return base + amp*math.Sin(t*9+seed)*math.Sin(t*3.1+seed*2)*(1-t*0.5)
}

func sessionStart(sym string, day time.Time) (time.Time, int) {
	d := day.In(Shanghai)
	at := func(h, m int) time.Time { return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, Shanghai) }
	switch MarketOf(sym) {
	case "US":
		return at(21, 30), 390
	case "HK":
		return at(9, 30), 330
	default:
		return at(9, 30), 240
	}
}

func (m *Mock) Intraday(_ context.Context, sym string) ([]Point, error) {
	if err := m.begin("Intraday"); err != nil {
		return nil, err
	}
	s, ok := mockSecOf(sym)
	if !ok {
		return nil, errors.New("没有这只股票")
	}
	start, n := sessionStart(s.Symbol, m.now())
	n = n * 7 / 10 // the day is under way
	seed := float64(len(s.Name))
	pts := make([]Point, n)
	sum := 0.0
	for i := range pts {
		p := wave(s.prev, s.last, i, n, seed)
		sum += p
		pts[i] = Point{Time: start.Add(time.Duration(i) * time.Minute), Price: p, Avg: sum / float64(i+1), Volume: int64(1000 + 800*math.Abs(math.Sin(float64(i)/7)))}
	}
	return pts, nil
}

func (m *Mock) Candles(_ context.Context, sym string, p Period, n int) ([]Candle, error) {
	if err := m.begin("Candles"); err != nil {
		return nil, err
	}
	s, ok := mockSecOf(sym)
	if !ok {
		return nil, errors.New("没有这只股票")
	}
	step := 24 * time.Hour
	switch p {
	case Week:
		step *= 7
	case Month:
		step *= 30
	}
	end := m.now()
	from := s.last * (0.7 + 0.1*float64(p))
	out := make([]Candle, n)
	seed := float64(len(s.Symbol))
	prevClose := from
	for i := range out {
		c := wave(from, s.last, i, n, seed)
		o := prevClose
		hi, lo := max(o, c)*(1+0.008+0.006*math.Abs(math.Sin(float64(i)))), min(o, c)*(1-0.008-0.005*math.Abs(math.Cos(float64(i))))
		out[i] = Candle{Time: end.Add(-time.Duration(n-1-i) * step), Open: o, High: hi, Low: lo, Close: c, Volume: int64(2e6 * (1 + math.Abs(math.Sin(float64(i)/3))))}
		prevClose = c
	}
	return out, nil
}

func (m *Mock) News(_ context.Context, sym string) ([]News, error) {
	if err := m.begin("News"); err != nil {
		return nil, err
	}
	s, _ := mockSecOf(sym)
	now := m.now()
	name := s.Name
	if name == "" {
		name = sym
	}
	return []News{
		{Title: "示例新闻：" + name + "发布季度业绩，营收同比增长", URL: "https://example.com/news/1", Time: now.Add(-40 * time.Minute)},
		{Title: "示例新闻：机构上调" + name + "目标价", URL: "https://example.com/news/2", Time: now.Add(-5 * time.Hour)},
		{Title: "示例新闻：" + name + "宣布新一轮回购计划", URL: "https://example.com/news/3", Time: now.Add(-28 * time.Hour)},
	}, nil
}

func (m *Mock) Markets(context.Context) ([]Market, error) {
	if err := m.begin("Markets"); err != nil {
		return nil, err
	}
	now := m.now()
	var out []Market
	for _, mk := range []string{"HK", "US", "CN"} {
		out = append(out, Market{Market: mk, Phase: ScheduledPhase(mk, now), At: now})
	}
	return out, nil
}

func (m *Mock) Directory(context.Context) ([]Watched, error) {
	if err := m.begin("Directory"); err != nil {
		return nil, err
	}
	var out []Watched
	for _, s := range mockSecs {
		if MarketOf(s.Symbol) == "US" {
			out = append(out, Watched{Symbol: s.Symbol, Name: s.Name + "\x00" + s.NameEn})
		}
	}
	return out, nil
}

func (m *Mock) Positions(context.Context) ([]Position, error) {
	if err := m.begin("Positions"); err != nil {
		return nil, err
	}
	return []Position{
		{Symbol: "700.HK", Name: "腾讯控股", Currency: "HKD", Quantity: 300, Available: 300, Cost: 388.2},
		{Symbol: "AAPL.US", Name: "苹果", Currency: "USD", Quantity: 40, Available: 40, Cost: 251.6},
		{Symbol: "NVDA.US", Name: "英伟达", Currency: "USD", Quantity: 60, Available: 60, Cost: 121.35},
		{Symbol: "600519.SH", Name: "贵州茅台", Currency: "CNY", Quantity: 100, Available: 100, Cost: 1520},
	}, nil
}

func (m *Mock) Balances(context.Context) ([]Balance, error) {
	if err := m.begin("Balances"); err != nil {
		return nil, err
	}
	return []Balance{{Currency: "HKD", NetAssets: 482_315.62, TotalCash: 61_208.4, BuyPower: 120_400,
		Cash: []Cash{{Currency: "HKD", Available: 38_120.5}, {Currency: "USD", Available: 2_960.12}}}}, nil
}

func (m *Mock) Subscribe(_ context.Context, syms []string) error {
	if err := m.begin("Subscribe"); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subscribed == nil {
		m.subscribed = map[string]bool{}
	}
	for _, s := range syms {
		m.subscribed[s] = true
	}
	return nil
}

func (m *Mock) Unsubscribe(_ context.Context, syms []string) error {
	if err := m.begin("Unsubscribe"); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range syms {
		delete(m.subscribed, s)
	}
	return nil
}

// Subscribed are the symbols subscribed.
func (m *Mock) Subscribed() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for k, v := range m.subscribed {
		out[k] = v
	}
	return out
}

func (m *Mock) OnPush(fn func(Push)) {
	m.mu.Lock()
	m.push = fn
	m.mu.Unlock()
}

func (m *Mock) OnReconnect(fn func(bool)) {
	m.mu.Lock()
	m.recon = fn
	m.mu.Unlock()
}

// Emit pushes p, as the server would, if its symbol is subscribed.
func (m *Mock) Emit(p Push) {
	m.mu.Lock()
	fn, ok := m.push, m.subscribed[p.Symbol]
	if s, found := mockSecOf(p.Symbol); found && p.Session == Regular {
		m.quote(s).Apply(p)
	}
	m.mu.Unlock()
	if fn != nil && ok {
		fn(p)
	}
}

// Reconnect tells, as the SDK would after the connection came back.
func (m *Mock) Reconnect(ok bool) {
	m.mu.Lock()
	fn := m.recon
	m.mu.Unlock()
	if fn != nil {
		fn(ok)
	}
}

// Live pushes small moves of the subscribed symbols every d, until ctx
// is done: the demo mode's ticking prices.
func (m *Mock) Live(ctx context.Context, d time.Duration) {
	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
		i++
		s := mockSecs[i%len(mockSecs)]
		m.mu.Lock()
		last := m.quote(s).Last
		m.mu.Unlock()
		step := last * 0.0007 * math.Sin(float64(i)*1.7)
		m.Emit(Push{Symbol: s.Symbol, Last: math.Round((last+step)*100) / 100, Time: m.now()})
	}
}

func (m *Mock) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}
