// Package feed is the market data LB Bar shows: quotes and their pushes,
// watchlists, charts, news, market status, and the account's positions
// and balances, read only. Provider is what the app needs of it; Longbridge
// (longbridge.go) is the real one, Mock (mock.go) the example data of tests
// and screenshots.
package feed

import (
	"context"
	"strings"
	"time"
)

// Shanghai is the user's time zone: every time the app shows is in it.
var Shanghai = time.FixedZone("CST", 8*3600)

// Quote is a security's latest quote. Prices are in its currency; 0 is
// unknown.
type Quote struct {
	Symbol    string
	Last      float64
	PrevClose float64
	Open      float64
	High      float64
	Low       float64
	Volume    int64
	Turnover  float64
	Time      time.Time
	Halted    bool // not trading: suspended, delisted…
	// US pre-market, post-market and overnight quotes, when there are.
	Pre, Post, Night *Ext
}

// Ext is a quote of a US session outside regular hours.
type Ext struct {
	Last      float64
	PrevClose float64
	High, Low float64
	Volume    int64
	Time      time.Time
}

// Change is the change since the previous close, and its rate (0.012 for +1.2%).
func (q Quote) Change() (abs, rate float64) {
	if q.PrevClose == 0 || q.Last == 0 {
		return 0, 0
	}
	return q.Last - q.PrevClose, (q.Last - q.PrevClose) / q.PrevClose
}

// Change of an extended session: against the regular close.
func (e Ext) Change() (abs, rate float64) {
	if e.PrevClose == 0 || e.Last == 0 {
		return 0, 0
	}
	return e.Last - e.PrevClose, (e.Last - e.PrevClose) / e.PrevClose
}

// Session is the trading session a push is of.
type Session int

const (
	Regular Session = iota
	PreMarket
	PostMarket
	Overnight
)

// Push is a quote pushed while subscribed: what changed.
type Push struct {
	Symbol   string
	Last     float64
	Open     float64
	High     float64
	Low      float64
	Volume   int64
	Turnover float64
	Time     time.Time
	Session  Session
}

// Apply merges a push into a quote: the regular session's into its
// fields, an extended session's into that session's quote.
func (q *Quote) Apply(p Push) {
	if p.Last == 0 {
		return
	}
	switch p.Session {
	case PreMarket, PostMarket, Overnight:
		ext := &q.Pre
		if p.Session == PostMarket {
			ext = &q.Post
		} else if p.Session == Overnight {
			ext = &q.Night
		}
		if *ext == nil {
			*ext = &Ext{PrevClose: q.Last}
		}
		e := *ext
		e.Last, e.Time = p.Last, p.Time
		if p.High > 0 {
			e.High = p.High
		}
		if p.Low > 0 {
			e.Low = p.Low
		}
		if p.Volume > 0 {
			e.Volume = p.Volume
		}
		return
	}
	q.Last, q.Time = p.Last, p.Time
	if p.Open > 0 {
		q.Open = p.Open
	}
	if p.High > 0 {
		q.High = p.High
	}
	if p.Low > 0 {
		q.Low = p.Low
	}
	if p.Volume > 0 {
		q.Volume = p.Volume
	}
	if p.Turnover > 0 {
		q.Turnover = p.Turnover
	}
}

// Security is what is known of a security that seldom changes.
type Security struct {
	Symbol      string
	Name        string // in Chinese, where there is one
	NameEn      string
	Currency    string
	LotSize     int
	TotalShares int64
	EPS         float64 // trailing twelve months
	BPS         float64
}

// Stats are a security's computed indexes.
type Stats struct {
	Symbol       string
	MarketCap    float64
	PE           float64 // TTM; 0 when there is none
	PB           float64
	TurnoverRate float64 // 0.0004 for 0.04%
}

// Point is a minute of the day's chart.
type Point struct {
	Time   time.Time
	Price  float64
	Avg    float64
	Volume int64
}

// Candle is a K-line bar.
type Candle struct {
	Time                   time.Time
	Open, High, Low, Close float64
	Volume                 int64
}

// Period is a K-line's.
type Period int

const (
	Day Period = iota
	Week
	Month
)

// Group is a watchlist group of the user's.
type Group struct {
	ID         int64
	Name       string
	Securities []Watched
}

// Watched is a security in a watchlist group.
type Watched struct {
	Symbol string
	Name   string
}

// News is a news item about a security.
type News struct {
	Title string
	URL   string
	Time  time.Time
}

// Position is a holding of the account.
type Position struct {
	Symbol    string
	Name      string
	Currency  string
	Quantity  float64
	Available float64
	Cost      float64 // the average cost price
}

// Balance is the account's assets in one currency.
type Balance struct {
	Currency  string
	NetAssets float64
	TotalCash float64
	BuyPower  float64
	Cash      []Cash
}

// Cash is the cash of one currency in an account.
type Cash struct {
	Currency  string
	Available float64
	Frozen    float64
	Settling  float64
}

// Phase is a market's trading state.
type Phase int

const (
	Closed Phase = iota
	Trading
	PreOpen   // pre-market (US), opening auction (HK, CN)
	PostClose // post-market (US), closing auction
	Break     // midday break
	Night     // US overnight
)

// Market is one market's state: HK, US, CN or SG.
type Market struct {
	Market string
	Phase  Phase
	At     time.Time
}

// Provider is what the app needs of the market data. All its methods but
// OnPush and OnReconnect may block on the network; the app calls them off
// the main thread. It never places, changes or cancels an order: there is
// nothing in it for that.
type Provider interface {
	Groups(ctx context.Context) ([]Group, error)
	Static(ctx context.Context, symbols []string) ([]Security, error)
	Quotes(ctx context.Context, symbols []string) ([]Quote, error)
	Stats(ctx context.Context, symbols []string) ([]Stats, error)
	Intraday(ctx context.Context, symbol string) ([]Point, error)
	Candles(ctx context.Context, symbol string, p Period, n int) ([]Candle, error)
	News(ctx context.Context, symbol string) ([]News, error)
	Markets(ctx context.Context) ([]Market, error)
	// Directory is a list of securities with their names to search
	// in: the US securities the API lists. It may be long.
	Directory(ctx context.Context) ([]Watched, error)
	Positions(ctx context.Context) ([]Position, error)
	Balances(ctx context.Context) ([]Balance, error)

	// Subscribe has the quotes of symbols pushed, to OnPush; the
	// subscription survives reconnects. Unsubscribe stops them.
	Subscribe(ctx context.Context, symbols []string) error
	Unsubscribe(ctx context.Context, symbols []string) error
	OnPush(fn func(Push))
	// OnReconnect is told when the push connection came back, and whether
	// the subscriptions did.
	OnReconnect(fn func(resubscribed bool))
	Close() error
}

// Market is the market of a symbol: "HK" of 700.HK; "CN" of .SH, .SZ.
func MarketOf(symbol string) string {
	i := strings.LastIndexByte(symbol, '.')
	if i < 0 {
		return ""
	}
	switch m := symbol[i+1:]; m {
	case "SH", "SZ", "BJ":
		return "CN"
	default:
		return m
	}
}

// Code is a symbol without its market: 700 of 700.HK.
func Code(symbol string) string {
	if i := strings.LastIndexByte(symbol, '.'); i > 0 {
		return symbol[:i]
	}
	return symbol
}
