package feed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	openapi "github.com/longbridge/openapi-go"
	"github.com/longbridge/openapi-go/config"
	"github.com/longbridge/openapi-go/content"
	lbhttp "github.com/longbridge/openapi-go/http"
	"github.com/longbridge/openapi-go/longbridge"
	"github.com/longbridge/openapi-go/market"
	"github.com/longbridge/openapi-go/quote"
	"github.com/longbridge/openapi-go/trade"
	"github.com/shopspring/decimal"
)

// Creds are the legacy API-key credentials of the developer center
// (open.longbridge.com → 用户中心 → 应用凭证).
type Creds struct {
	AppKey      string `json:"appKey"`
	AppSecret   string `json:"appSecret"`
	AccessToken string `json:"accessToken"`
}

// Complete tells whether all three are there.
func (c Creds) Complete() bool {
	return strings.TrimSpace(c.AppKey) != "" && strings.TrimSpace(c.AppSecret) != "" && strings.TrimSpace(c.AccessToken) != ""
}

// Options choose the endpoints.
type Options struct {
	// Region "cn" uses the mainland China endpoints (*.longbridge.cn),
	// faster from the mainland; "" the global ones.
	Region    string
	Overnight bool // US overnight quotes
}

// The mainland endpoints, as the SDK has them for LONGPORT_REGION=cn.
const (
	cnHTTP  = "https://openapi.longbridge.cn"
	cnQuote = "wss://openapi-quote.longbridge.cn"
	cnTrade = "wss://openapi-trade.longbridge.cn"
)

// Longbridge is the Provider on the Longbridge OpenAPI. Its trade
// context is only ever asked for positions and balances.
type Longbridge struct {
	cfg     *config.Config
	q       *quote.QuoteContext
	mkt     *market.MarketContext
	news    *content.ContentContext
	limit   *Limiter
	mu      sync.Mutex
	onPush  func(Push)
	onRecon func(bool)
	trade   *trade.TradeContext
}

// Dial connects to the quote server with creds: an error is mostly
// wrong or expired credentials, or no network.
func Dial(creds Creds, o Options) (*Longbridge, error) {
	if !creds.Complete() {
		return nil, errors.New("缺少 App Key、App Secret 或 Access Token")
	}
	cfg, err := config.New(config.WithConfigKey(strings.TrimSpace(creds.AppKey), strings.TrimSpace(creds.AppSecret), strings.TrimSpace(creds.AccessToken)))
	if err != nil {
		return nil, err
	}
	if o.Region == "cn" {
		cfg.HttpURL, cfg.QuoteUrl, cfg.TradeUrl = cnHTTP, cnQuote, cnTrade
	}
	cfg.EnableOvernight = o.Overnight
	cfg.Language = openapi.LanguageZHCN
	cfg.LogLevel = "error"
	l := &Longbridge{cfg: cfg, limit: NewLimiter()}
	hc, err := lbhttp.NewFromCfg(cfg)
	if err != nil {
		return nil, Friendly(err)
	}
	// quote.NewFromCfg, with a reconnect callback.
	q, err := quote.New(
		quote.WithQuoteURL(cfg.QuoteUrl),
		quote.WithHttpClient(hc),
		quote.WithLbOptions(longbridge.NewOptions(
			longbridge.WithAuthTimeout(cfg.AuthTimeout),
			longbridge.WithTimeout(cfg.Timeout),
			longbridge.WithReadBufferSize(cfg.ReadBufferSize),
			longbridge.WithReadQueueSize(cfg.ReadQueueSize),
			longbridge.WithWriteQueueSize(cfg.WriteQueueSize),
			longbridge.WithMinGzipSize(cfg.MinGzipSize),
		)),
		quote.WithLogLevel(cfg.LogLevel),
		quote.WithEnableOvernight(cfg.EnableOvernight),
		quote.WithLanguage(cfg.Language),
		quote.OnReconnect(l.reconnected),
	)
	if err != nil {
		err = Friendly(err)
		return nil, err
	}
	l.q = q
	q.OnQuote(l.pushed)
	if l.mkt, err = market.NewFromCfg(cfg); err != nil {
		q.Close()
		return nil, err
	}
	if l.news, err = content.NewFromCfg(cfg); err != nil {
		q.Close()
		return nil, err
	}
	return l, nil
}

func (l *Longbridge) pushed(p *quote.PushQuote) {
	l.mu.Lock()
	fn := l.onPush
	l.mu.Unlock()
	if fn == nil || p == nil {
		return
	}
	s := Regular
	switch p.TradeSession {
	case quote.TradeSessionType(quote.TradeSessionPreTrade):
		s = PreMarket
	case quote.TradeSessionType(quote.TradeSessionPostTrade):
		s = PostMarket
	case quote.TradeSessionType(quote.TradeSessionOvernight):
		s = Overnight
	}
	fn(Push{Symbol: p.Symbol, Last: f(p.LastDone), Open: f(p.Open), High: f(p.High), Low: f(p.Low),
		Volume: p.Volume, Turnover: f(p.Turnover), Time: unix(p.Timestamp), Session: s})
}

func (l *Longbridge) OnPush(fn func(Push)) {
	l.mu.Lock()
	l.onPush = fn
	l.mu.Unlock()
}

func (l *Longbridge) reconnected(ok bool) {
	l.mu.Lock()
	fn := l.onRecon
	l.mu.Unlock()
	if fn != nil {
		fn(ok)
	}
}

// OnReconnect: the SDK reconnects and resubscribes by itself, and tells.
func (l *Longbridge) OnReconnect(fn func(bool)) {
	l.mu.Lock()
	l.onRecon = fn
	l.mu.Unlock()
}

// call runs fn within the rate limit and a timeout.
func (l *Longbridge) call(ctx context.Context, fn func(ctx context.Context) error) error {
	done, err := l.limit.Wait(ctx)
	if err != nil {
		return err
	}
	defer done()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return Friendly(fn(ctx))
}

func (l *Longbridge) Groups(ctx context.Context) (out []Group, err error) {
	err = l.call(ctx, func(ctx context.Context) error {
		gs, err := l.q.WatchedGroups(ctx)
		for _, g := range gs {
			gr := Group{ID: g.Id, Name: g.Name}
			for _, s := range g.Securites {
				gr.Securities = append(gr.Securities, Watched{Symbol: s.Symbol, Name: s.Name})
			}
			out = append(out, gr)
		}
		return err
	})
	return
}

// batches calls fn with symbols, up to n at a time.
func batches(symbols []string, n int, fn func([]string) error) error {
	for i := 0; i < len(symbols); i += n {
		if err := fn(symbols[i:min(i+n, len(symbols))]); err != nil {
			return err
		}
	}
	return nil
}

func (l *Longbridge) Static(ctx context.Context, symbols []string) (out []Security, err error) {
	err = batches(symbols, 500, func(b []string) error {
		return l.call(ctx, func(ctx context.Context) error {
			ss, err := l.q.StaticInfo(ctx, b)
			for _, s := range ss {
				name := s.NameCn
				if name == "" {
					name = s.NameHk
				}
				if name == "" {
					name = s.NameEn
				}
				out = append(out, Security{Symbol: s.Symbol, Name: name, NameEn: s.NameEn, Currency: s.Currency,
					LotSize: int(s.LotSize), TotalShares: s.TotalShares, EPS: f(s.EpsTtm), BPS: f(s.Bps)})
			}
			return err
		})
	})
	return
}

func ext(p *quote.PrePostQuote) *Ext {
	if p == nil || f(p.LastDone) == 0 {
		return nil
	}
	return &Ext{Last: f(p.LastDone), PrevClose: f(p.PrevClose), High: f(p.High), Low: f(p.Low), Volume: p.Volume, Time: unix(p.Timestamp)}
}

func (l *Longbridge) Quotes(ctx context.Context, symbols []string) (out []Quote, err error) {
	err = batches(symbols, 500, func(b []string) error {
		return l.call(ctx, func(ctx context.Context) error {
			qs, err := l.q.Quote(ctx, b)
			for _, q := range qs {
				out = append(out, Quote{Symbol: q.Symbol, Last: f(q.LastDone), PrevClose: f(q.PrevClose), Open: f(q.Open),
					High: f(q.High), Low: f(q.Low), Volume: q.Volume, Turnover: f(q.Turnover), Time: unix(q.Timestamp),
					Halted: q.TradeStatus != 0, Pre: ext(q.PreMarketQuote), Post: ext(q.PostMarketQuote), Night: ext(q.OverNightQuote)})
			}
			return err
		})
	})
	return
}

func (l *Longbridge) Stats(ctx context.Context, symbols []string) (out []Stats, err error) {
	err = batches(symbols, 500, func(b []string) error {
		return l.call(ctx, func(ctx context.Context) error {
			cs, err := l.q.CalcIndex(ctx, b, []quote.CalcIndex{quote.CalcIndexTotalMarketValue, quote.CalcIndexPeTTMRatio, quote.CalcIndexPbRatio, quote.CalcIndexTurnoverRate})
			for _, c := range cs {
				out = append(out, Stats{Symbol: c.Symbol, MarketCap: f(c.TotalMarketValue), PE: f(c.PeTtmRatio), PB: f(c.PbRatio), TurnoverRate: f(c.TurnoverRate) / 100})
			}
			return err
		})
	})
	return
}

func (l *Longbridge) Intraday(ctx context.Context, symbol string) (out []Point, err error) {
	err = l.call(ctx, func(ctx context.Context) error {
		ls, err := l.q.Intraday(ctx, symbol)
		for _, p := range ls {
			out = append(out, Point{Time: unix(p.Timestamp), Price: f(p.Price), Avg: f(p.AvgPrice), Volume: p.Volume})
		}
		return err
	})
	return
}

func (l *Longbridge) Candles(ctx context.Context, symbol string, p Period, n int) (out []Candle, err error) {
	period := quote.PeriodDay
	switch p {
	case Week:
		period = quote.PeriodWeek
	case Month:
		period = quote.PeriodMonth
	}
	err = l.call(ctx, func(ctx context.Context) error {
		cs, err := l.q.Candlesticks(ctx, symbol, period, int32(n), quote.AdjustTypeForward)
		for _, c := range cs {
			out = append(out, Candle{Time: unix(c.Timestamp), Open: f(c.Open), High: f(c.High), Low: f(c.Low), Close: f(c.Close), Volume: c.Volume})
		}
		return err
	})
	return
}

func (l *Longbridge) News(ctx context.Context, symbol string) (out []News, err error) {
	err = l.call(ctx, func(ctx context.Context) error {
		ns, err := l.news.News(ctx, symbol)
		for _, n := range ns {
			out = append(out, News{Title: n.Title, URL: n.Url, Time: n.PublishedAt})
		}
		return err
	})
	return
}

func (l *Longbridge) Markets(ctx context.Context) (out []Market, err error) {
	err = l.call(ctx, func(ctx context.Context) error {
		r, err := l.mkt.MarketStatus(ctx)
		if r != nil {
			for _, m := range r.MarketTime {
				out = append(out, Market{Market: m.Market, Phase: phaseOf(m.TradeStatus), At: m.Timestamp})
			}
		}
		return err
	})
	return
}

func phaseOf(s market.TradeStatus) Phase {
	switch s {
	case market.TradeStatusTrading, market.TradeStatusUSTrading, market.TradeStatusDarkTrading:
		return Trading
	case market.TradeStatusOpenBid, market.TradeStatusUSPrev, market.TradeStatusUSClean, market.TradeStatusUSPrevMarketClean, market.TradeStatusDarkWait:
		return PreOpen
	case market.TradeStatusCloseBid, market.TradeStatusUSAfter, market.TradeStatusUSAfterMarketClean, market.TradeStatusAfterFix:
		return PostClose
	case market.TradeStatusNoonClosing, market.TradeStatusMorningClosing, market.TradeStatusRealtimeQuote:
		return Break
	case market.TradeStatusUSNight:
		return Night
	}
	return Closed
}

func (l *Longbridge) Directory(ctx context.Context) (out []Watched, err error) {
	err = l.call(ctx, func(ctx context.Context) error {
		ss, err := l.q.SecurityList(ctx, openapi.MarketUS, quote.Overnight)
		for _, s := range ss {
			name := s.NameCN
			if name == "" {
				name = s.NameEN
			}
			out = append(out, Watched{Symbol: s.Symbol, Name: name + "\x00" + s.NameEN})
		}
		return err
	})
	return
}

// tradeCtx is the trade context, made at first use: for positions and
// balances only.
func (l *Longbridge) tradeCtx() (*trade.TradeContext, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.trade != nil {
		return l.trade, nil
	}
	t, err := trade.NewFromCfg(l.cfg)
	if err != nil {
		return nil, Friendly(err)
	}
	l.trade = t
	return t, nil
}

func (l *Longbridge) Positions(ctx context.Context) (out []Position, err error) {
	t, err := l.tradeCtx()
	if err != nil {
		return nil, err
	}
	err = l.call(ctx, func(ctx context.Context) error {
		chs, err := t.StockPositions(ctx, nil)
		for _, ch := range chs {
			for _, p := range ch.Positions {
				out = append(out, Position{Symbol: p.Symbol, Name: p.SymbolName, Currency: p.Currency,
					Quantity: num(p.Quantity), Available: num(p.AvailableQuantity), Cost: f(p.CostPrice)})
			}
		}
		return err
	})
	return
}

func (l *Longbridge) Balances(ctx context.Context) (out []Balance, err error) {
	t, err := l.tradeCtx()
	if err != nil {
		return nil, err
	}
	err = l.call(ctx, func(ctx context.Context) error {
		bs, err := t.AccountBalance(ctx, &trade.GetAccountBalance{})
		for _, b := range bs {
			bal := Balance{Currency: b.Currency, NetAssets: f(b.NetAssets), TotalCash: f(b.TotalCash), BuyPower: f(b.BuyPower)}
			for _, c := range b.CashInfos {
				bal.Cash = append(bal.Cash, Cash{Currency: c.Currency, Available: f(c.AvailableCash), Frozen: f(c.FrozenCash), Settling: f(c.SettlingCash)})
			}
			out = append(out, bal)
		}
		return err
	})
	return
}

func (l *Longbridge) Subscribe(ctx context.Context, symbols []string) error {
	return batches(symbols, 100, func(b []string) error {
		return l.call(ctx, func(ctx context.Context) error {
			return l.q.Subscribe(ctx, b, []quote.SubType{quote.SubTypeQuote}, false)
		})
	})
}

func (l *Longbridge) Unsubscribe(ctx context.Context, symbols []string) error {
	return batches(symbols, 100, func(b []string) error {
		return l.call(ctx, func(ctx context.Context) error {
			return l.q.Unsubscribe(ctx, false, b, []quote.SubType{quote.SubTypeQuote})
		})
	})
}

func (l *Longbridge) Close() error {
	l.mu.Lock()
	t := l.trade
	l.trade = nil
	l.mu.Unlock()
	if t != nil {
		_ = t.Close()
	}
	return l.q.Close()
}

func f(d *decimal.Decimal) float64 {
	if d == nil {
		return 0
	}
	v, _ := d.Float64()
	return v
}

func num(s string) float64 {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	v, _ := d.Float64()
	return v
}

func unix(s int64) time.Time {
	if s == 0 {
		return time.Time{}
	}
	return time.Unix(s, 0)
}

// Friendly says an API error in Chinese where it knows it, and never
// with the credentials in it.
func Friendly(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(low, "timeout"):
		return fmt.Errorf("连接长桥超时，检查网络，或在设置里换一条线路")
	case strings.Contains(low, "401") || strings.Contains(low, "unauthorized") || strings.Contains(low, "token") && (strings.Contains(low, "expire") || strings.Contains(low, "invalid")):
		return fmt.Errorf("凭证无效或已过期（%s）", short(msg))
	case strings.Contains(low, "429") || strings.Contains(low, "rate limit") || strings.Contains(low, "too many"):
		return fmt.Errorf("请求太频繁，稍后会自动重试")
	case strings.Contains(low, "no such host") || strings.Contains(low, "connection refused") || strings.Contains(low, "network is unreachable"):
		return fmt.Errorf("连不上长桥服务器，检查网络")
	}
	return errors.New(short(msg))
}

// short is an error's last, most telling part, at most 120 characters.
func short(s string) string {
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120]) + "…"
	}
	return s
}
