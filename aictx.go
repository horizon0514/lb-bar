package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"lb-bar/dsapi"
	"lb-bar/feed"
)

// What a conversation about a security tells the model, once, at its
// start: the quote, the indicators, the day's chart in brief, the latest
// Longbridge news, and, only when the user allowed it, the holdings. The
// cash and the account's assets are never sent.

// maxContextNews is how many headlines a conversation starts with.
const maxContextNews = 5

// aiInput is what the context is built of, gathered on the main thread
// and completed from the API on the conversation's goroutine.
type aiInput struct {
	Symbol    string
	Sec       feed.Security
	Quote     *feed.Quote
	Stats     *feed.Stats
	Intraday  []feed.Point
	News      []feed.News
	NewsAsked bool
	Market    string // its phase, as the app shows it: 港股 交易中
	Now       time.Time

	// Positions are the holdings: only with the user's leave
	// (config.AIPositions), else nil, whatever the app has loaded.
	AllowPositions bool
	Positions      []feed.Position
	PosQuotes      map[string]feed.Quote
}

// aiContext is what was sent: its text, its parts (the chip: 报价、指标、
// 新闻 5 条), and the sources [1]… the news are, for citations.
type aiContext struct {
	Text    string
	Parts   []string
	Sources []dsapi.Result
}

// buildContext writes the context of in. It is pure: the tests check
// what it sends, and what it doesn't.
func buildContext(in aiInput) aiContext {
	var b strings.Builder
	var out aiContext
	now := in.Now.In(feed.Shanghai)
	name := in.Sec.Name
	if name == "" {
		name = feed.Code(in.Symbol)
	}
	cur := in.Sec.Currency
	fmt.Fprintf(&b, "【行情上下文】数据来自长桥 OpenAPI，北京时间 %s 获取。\n", now.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "证券：%s（%s，%s", name, in.Symbol, feed.MarketName(feed.MarketOf(in.Symbol)))
	if in.Sec.NameEn != "" && in.Sec.NameEn != name {
		fmt.Fprintf(&b, "，英文名 %s", in.Sec.NameEn)
	}
	if cur != "" {
		fmt.Fprintf(&b, "，币种 %s", cur)
	}
	b.WriteString("）\n")
	if in.Market != "" {
		fmt.Fprintf(&b, "市场状态：%s\n", in.Market)
	}

	if q := in.Quote; q != nil && q.Last > 0 {
		out.Parts = append(out.Parts, "报价")
		abs, rate := q.Change()
		fmt.Fprintf(&b, "最新价：%s，涨跌 %s（%s），昨收 %s；今开 %s，最高 %s，最低 %s", price(q.Last), signed(abs), pct(rate), price(q.PrevClose), price(q.Open), price(q.High), price(q.Low))
		if !q.Time.IsZero() {
			fmt.Fprintf(&b, "；报价时间 %s", q.Time.In(feed.Shanghai).Format("01-02 15:04"))
		}
		b.WriteString("\n")
		if q.Volume > 0 || q.Turnover > 0 {
			fmt.Fprintf(&b, "成交量 %s股，成交额 %s%s\n", big(float64(q.Volume)), big(q.Turnover), parens(cur))
		}
		if q.Halted {
			b.WriteString("注意：该证券当前不在正常交易状态（停牌或其他）。\n")
		}
		for _, e := range []struct {
			label string
			x     *feed.Ext
		}{{"盘前", q.Pre}, {"盘后", q.Post}, {"夜盘", q.Night}} {
			if e.x == nil || e.x.Last == 0 {
				continue
			}
			ea, er := e.x.Change()
			fmt.Fprintf(&b, "美股%s：%s，涨跌 %s（%s，相对 %s）", e.label, price(e.x.Last), signed(ea), pct(er), price(e.x.PrevClose))
			if !e.x.Time.IsZero() {
				fmt.Fprintf(&b, "，北京时间 %s", e.x.Time.In(feed.Shanghai).Format("01-02 15:04"))
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("最新价：暂无\n")
	}

	var ind []string
	if st := in.Stats; st != nil {
		if st.MarketCap > 0 {
			ind = append(ind, "总市值 "+currencySign(cur)+big(st.MarketCap))
		}
		switch {
		case st.PE > 0:
			ind = append(ind, fmt.Sprintf("市盈率TTM %.2f", st.PE))
		case st.PE < 0:
			ind = append(ind, "市盈率TTM 为负（亏损）")
		}
		if st.PB > 0 {
			ind = append(ind, fmt.Sprintf("市净率 %.2f", st.PB))
		}
		if st.TurnoverRate > 0 {
			ind = append(ind, fmt.Sprintf("换手率 %.2f%%", st.TurnoverRate*100))
		}
	}
	if in.Sec.EPS != 0 {
		ind = append(ind, fmt.Sprintf("每股收益TTM %.3f", in.Sec.EPS))
	}
	if in.Sec.BPS != 0 {
		ind = append(ind, fmt.Sprintf("每股净资产 %.3f", in.Sec.BPS))
	}
	if in.Sec.TotalShares > 0 {
		ind = append(ind, "总股本 "+big(float64(in.Sec.TotalShares))+"股")
	}
	if in.Sec.LotSize > 0 {
		ind = append(ind, fmt.Sprintf("每手 %d 股", in.Sec.LotSize))
	}
	if len(ind) > 0 {
		out.Parts = append(out.Parts, "指标")
		b.WriteString("指标：" + strings.Join(ind, "，") + "\n")
	}

	if s := intradaySummary(in.Intraday, in.Quote); s != "" {
		out.Parts = append(out.Parts, "分时")
		b.WriteString(s)
	}

	ns := append([]feed.News(nil), in.News...)
	sort.SliceStable(ns, func(i, j int) bool { return ns[i].Time.After(ns[j].Time) })
	ns = ns[:min(len(ns), maxContextNews)]
	if len(ns) > 0 {
		out.Parts = append(out.Parts, fmt.Sprintf("新闻 %d 条", len(ns)))
		fmt.Fprintf(&b, "长桥资讯最近 %d 条新闻（可引用编号）：\n", len(ns))
		for i, n := range ns {
			date := "日期未知"
			if !n.Time.IsZero() {
				date = n.Time.In(feed.Shanghai).Format("2006-01-02 15:04")
			}
			fmt.Fprintf(&b, "[%d] %s %s\n", i+1, date, n.Title)
			r := dsapi.Result{Title: n.Title, URL: n.URL, SiteName: "长桥资讯"}
			if !n.Time.IsZero() {
				d := n.Time.UTC().Format(time.RFC3339)
				r.Date = &d
			}
			out.Sources = append(out.Sources, r)
		}
	} else if in.NewsAsked {
		b.WriteString("长桥资讯：暂无相关新闻。\n")
	}

	if in.AllowPositions {
		if s, n := positionsSummary(in.Positions, in.PosQuotes); n > 0 {
			out.Parts = append(out.Parts, fmt.Sprintf("持仓 %d 只", n))
			b.WriteString(s)
		} else {
			b.WriteString("持仓：用户允许读取，但账户没有持仓。\n")
		}
	} else {
		b.WriteString("持仓：未附带（用户没有允许 AI 读取持仓；问到持仓时请提示可在 设置 › AI 里开启）。\n")
	}
	out.Text = b.String()
	return out
}

func parens(s string) string {
	if s == "" {
		return ""
	}
	return "（" + s + "）"
}

// intradaySummary is the day's chart in a few lines: the open, the high
// and low and when, the average price, and the price every half hour.
func intradaySummary(pts []feed.Point, q *feed.Quote) string {
	if len(pts) < 2 {
		return ""
	}
	first, last := pts[0], pts[len(pts)-1]
	hi, lo := first, first
	var maxVol feed.Point
	for _, p := range pts {
		if p.Price > hi.Price {
			hi = p
		}
		if p.Price < lo.Price {
			lo = p
		}
		if p.Volume > maxVol.Volume {
			maxVol = p
		}
	}
	hm := func(t time.Time) string { return t.In(feed.Shanghai).Format("15:04") }
	var b strings.Builder
	fmt.Fprintf(&b, "分时（%s，北京时间 %s–%s，%d 分钟）：首分钟 %s，最新 %s；最高 %s（%s），最低 %s（%s）",
		first.Time.In(feed.Shanghai).Format("2006-01-02"), hm(first.Time), hm(last.Time), len(pts),
		price(first.Price), price(last.Price), price(hi.Price), hm(hi.Time), price(lo.Price), hm(lo.Time))
	if last.Avg > 0 {
		d := (last.Price - last.Avg) / last.Avg
		where := "高于"
		if d < 0 {
			where = "低于"
		}
		fmt.Fprintf(&b, "；均价 %s，最新价%s均价 %.2f%%", price(last.Avg), where, math.Abs(d)*100)
	}
	if maxVol.Volume > 0 {
		fmt.Fprintf(&b, "；成交最大的一分钟 %s", hm(maxVol.Time))
	}
	b.WriteString("\n走势采样：")
	var samples []string
	lastAt := time.Time{}
	for i, p := range pts {
		if i == 0 || i == len(pts)-1 || p.Time.Sub(lastAt) >= 30*time.Minute {
			samples = append(samples, hm(p.Time)+" "+price(p.Price))
			lastAt = p.Time
		}
	}
	b.WriteString(strings.Join(samples, "，") + "\n")
	return b.String()
}

// positionsSummary is the holdings: quantity, cost, value and P/L each,
// and the P/L by currency. No cash, no assets.
func positionsSummary(ps []feed.Position, qs map[string]feed.Quote) (string, int) {
	if len(ps) == 0 {
		return "", 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "持仓（用户允许 AI 读取；共 %d 只，不含现金与资产）：\n", len(ps))
	for _, p := range ps {
		name := p.Name
		if name == "" {
			name = feed.Code(p.Symbol)
		}
		fmt.Fprintf(&b, "- %s（%s）%s 股，成本 %s", name, p.Symbol, trimQty(p.Quantity), price(p.Cost))
		if q, ok := qs[p.Symbol]; ok && q.Last > 0 {
			value := p.Quantity * q.Last
			fmt.Fprintf(&b, "，现价 %s，市值 %s %s", price(q.Last), p.Currency, money(value))
			if p.Cost > 0 {
				fmt.Fprintf(&b, "，累计盈亏 %s（%s）", signedMoney(value-p.Quantity*p.Cost), pct((q.Last-p.Cost)/p.Cost))
			}
			if q.PrevClose > 0 {
				fmt.Fprintf(&b, "，今日盈亏 %s", signedMoney(p.Quantity*(q.Last-q.PrevClose)))
			}
		}
		b.WriteString("\n")
	}
	return b.String(), len(ps)
}

// aiSystem is the system prompt of a conversation about a security.
func aiSystem(now time.Time, name, symbol string) string {
	return "你是「LB Bar 长桥行情栏」里的股票助手。今天是 " + now.In(feed.Shanghai).Format("2006-01-02") + "（北京时间）。" +
		"用户正在看 " + name + "（" + symbol + "）。用户消息里的【行情上下文】来自长桥 OpenAPI，是实时数据，回答以它为准，不要编造数字；数据没有的就直说。" +
		"用简体中文简洁回答（一般 4～8 句，或几条要点，可用少量 Markdown 加粗/列表）。" +
		"引用新闻或搜索结果中的事实时，在句末用 [n] 标注来源编号，可多个如 [1][6]。" +
		"需要上下文之外的信息（公司公告、财报、业绩指引、行业或宏观消息、涨跌原因）时调用 web_search，每轮最多 2 次；它返回的结果接着已有编号继续编号。" +
		"不给出确定的买卖指令、目标价或收益承诺；涉及判断时说明依据与不确定性。不要写开场白，不要复述问题。"
}

// stockSearchTool is web_search, described for questions about a stock.
var stockSearchTool = dsapi.Tool{Type: "function", Function: dsapi.FunctionDef{
	Name: "web_search",
	Description: "用 DeepSeek 搜索引擎搜索网页。当行情上下文和已有新闻不足以回答时调用，例如需要公司公告、财报、业绩指引、行业或宏观消息、今天涨跌的原因。" +
		"返回带编号的结果，编号接着已有来源继续；回答时用 [n] 引用。",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "搜索词，简洁，含公司名或代码，可用 after:2026-10-01 等语法，最多 128 字"},
			"count": map[string]any{"type": "integer", "description": "要几条结果，1–10，默认 6"},
		},
		"required": []string{"query"},
	},
}}

// todayPrompt is the quick action's question (⌘J): 解读今日走势.
const todayPrompt = "请解读这只股票今天（最近一个交易日）的走势：\n" +
	"1. 走势特征：开盘、盘中高低点及出现时间、当前或收盘位置、相对均价与昨收、成交是否放量；\n" +
	"2. 可能的原因：结合上面的新闻，必要时联网搜索今天的消息，引用来源编号；分清已证实的消息与推测；\n" +
	"3. 接下来值得关注的点（如关键价位、待公布的事件）。\n" +
	"控制在 200 字左右，不要给买卖建议。"

const todayLabel = "解读今日走势"

// followUpNote is the latest quote, added to a follow-up question: the
// conversation's context ages as prices move.
func followUpNote(q *feed.Quote, now time.Time) string {
	if q == nil || q.Last <= 0 {
		return ""
	}
	_, rate := q.Change()
	return fmt.Sprintf("\n\n（最新行情：现价 %s，涨跌 %s，北京时间 %s）", price(q.Last), pct(rate), now.In(feed.Shanghai).Format("15:04"))
}
