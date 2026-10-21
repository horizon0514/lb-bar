package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"lb-bar/billing"
	"lb-bar/dsapi"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui/markdown"
)

// The AI pane: a conversation with DeepSeek about the security of the
// detail, which starts with its context (aictx.go). The model may search
// the web, twice a turn at most. One conversation per security, in
// memory only.

const (
	maxSearchesPerTurn = 2
	maxCallsPerTurn    = 4 // chat completions a turn, the tool rounds in
	toolResults        = 6
)

// aiSource is DeepSeek: dsapi.Client, or a fake in tests.
type aiSource interface {
	Chat(ctx context.Context, msgs []dsapi.Message, tools []dsapi.Tool, onDelta func(string)) (dsapi.Reply, error)
	Search(ctx context.Context, query string, count int) ([]dsapi.Result, error)
	GetBalance(ctx context.Context) (dsapi.AccountBalance, error)
	Check(ctx context.Context) error
	ModelName() string
}

func newDeepSeek(key string) aiSource { return dsapi.New(key) }

// conv is a conversation about a security.
type conv struct {
	symbol, name string
	msgs         []dsapi.Message // as the API has them, once a turn is done
	turns        []*turn
	ctx          *aiContext // what it was told, once gathered
	sources      []dsapi.Result
	cancel       context.CancelFunc
}

// turn is a question and its answer.
type turn struct {
	question string // as shown
	prompt   string // as sent, when not the question (the quick action)
	searches []*toolSearch
	answer   string
	busy     bool
	err      string
	calls    int
	cost     float64 // yuan, estimated: completions and searches
	md       markdown.Options
}

type toolSearch struct {
	query string
	n     int
	busy  bool
	err   string
}

func (cv *conv) busy() bool { return len(cv.turns) > 0 && cv.turns[len(cv.turns)-1].busy }

// aiReady tells whether the AI can answer: a DeepSeek key.
func (a *app) aiReady() bool { return a.ai != nil }

// aiPane tells whether the expanded detail has the AI pane.
func (a *app) aiPane() bool { return !a.cfg.NoAIPane }

// chat is the conversation of the detail's security, or nil.
func (a *app) chat() *conv { return a.convs[a.detailSym] }

// newChat (⌘N in the AI pane) starts the conversation over.
func (a *app) newChat() {
	if cv := a.chat(); cv != nil && cv.cancel != nil {
		cv.cancel()
	}
	delete(a.convs, a.detailSym)
	a.aiDraft, a.showCtx = "", false
	a.focusAI = true
}

// quickAction (⌘J, the button) asks 解读今日走势 of the chosen security.
func (a *app) quickAction() {
	sym := a.detailSym
	if !a.expanded || sym == "" {
		sym = a.chosenSym()
	}
	if sym == "" {
		return
	}
	if !a.expanded || a.detailSym != sym {
		a.expand(true)
	}
	if a.cfg.NoAIPane {
		a.cfg.NoAIPane = false
		a.changed(a.cfg)
		a.resize(true)
	}
	a.focusAI = true
	a.askAI(todayLabel, todayPrompt)
}

// askAI asks q of the detail's security; prompt, when not "", is what is
// sent instead of q. A conversation's first question gathers its context.
func (a *app) askAI(q, prompt string) bool {
	q = strings.TrimSpace(q)
	sym := a.detailSym
	if q == "" || sym == "" {
		return false
	}
	if !a.aiReady() {
		a.toast = "先在 设置 › DeepSeek 填入 API Key"
		return false
	}
	cv := a.convs[sym]
	if cv != nil && cv.busy() {
		return false
	}
	if cv == nil {
		cv = &conv{symbol: sym, name: a.name(sym)}
		a.convs[sym] = cv
	}
	t := &turn{question: q, prompt: prompt}
	t.md.OnLink = func(dest string) { a.cite(cv, dest) }
	cv.turns = append(cv.turns, t)
	if prompt == "" {
		prompt = q
		a.lastQuestion = q
	}
	first := cv.ctx == nil
	var msgs []dsapi.Message
	if !first {
		msgs = append(slices.Clone(cv.msgs), dsapi.Message{Role: "user", Content: prompt + followUpNote(a.quotes[sym], a.now())})
	}
	in := a.aiGather(sym)
	a.runTurn(cv, t, msgs, in, prompt)
	return true
}

// aiGather is what the app has for the context of sym: copies, for the
// conversation's goroutine.
func (a *app) aiGather(sym string) aiInput {
	in := aiInput{Symbol: sym, Sec: a.secs[sym], Now: a.now(), AllowPositions: a.cfg.AIPositions}
	if in.Sec.Name == "" {
		in.Sec.Name = a.name(sym)
	}
	if q := a.quotes[sym]; q != nil {
		cp := *q
		in.Quote = &cp
	}
	if st, ok := a.stats[sym]; ok {
		in.Stats = &st
	}
	in.Intraday = slices.Clone(a.intraday[sym])
	ns, asked := a.news[sym]
	in.News, in.NewsAsked = slices.Clone(ns), asked
	m := feed.MarketOf(sym)
	in.Market = feed.MarketName(m) + " " + a.phase(m).Label(m)
	if in.AllowPositions && len(a.positions) > 0 && a.now().Sub(a.posAt) < 5*time.Minute {
		in.Positions = slices.Clone(a.positions)
		in.PosQuotes = map[string]feed.Quote{}
		for _, p := range in.Positions {
			if q := a.quotes[p.Symbol]; q != nil {
				in.PosQuotes[p.Symbol] = *q
			}
		}
	}
	return in
}

// gather completes in from the API, on the conversation's goroutine:
// what the detail had not loaded yet, and the holdings when allowed.
func gather(ctx context.Context, p feed.Provider, in aiInput) aiInput {
	if p == nil {
		return in
	}
	sym := []string{in.Symbol}
	if in.Quote == nil || in.Quote.Last == 0 {
		if qs, err := p.Quotes(ctx, sym); err == nil && len(qs) > 0 {
			in.Quote = &qs[0]
		}
	}
	if in.Stats == nil {
		if st, err := p.Stats(ctx, sym); err == nil && len(st) > 0 {
			in.Stats = &st[0]
		}
	}
	if in.Sec.Currency == "" {
		if ss, err := p.Static(ctx, sym); err == nil && len(ss) > 0 {
			name := in.Sec.Name
			in.Sec = ss[0]
			if in.Sec.Name == "" {
				in.Sec.Name = name
			}
		}
	}
	if len(in.Intraday) == 0 {
		if pts, err := p.Intraday(ctx, in.Symbol); err == nil {
			in.Intraday = pts
		}
	}
	if !in.NewsAsked {
		if ns, err := p.News(ctx, in.Symbol); err == nil {
			in.News, in.NewsAsked = ns, true
		}
	}
	if in.AllowPositions && in.Positions == nil {
		if ps, err := p.Positions(ctx); err == nil {
			in.Positions = ps
			in.PosQuotes = map[string]feed.Quote{}
			var syms []string
			for _, x := range ps {
				syms = append(syms, x.Symbol)
			}
			if len(syms) > 0 {
				if qs, err := p.Quotes(ctx, syms); err == nil {
					for _, q := range qs {
						in.PosQuotes[q.Symbol] = q
					}
				}
			}
		}
	}
	return in
}

// runTurn answers t: the context first, on a conversation's first turn;
// then completions in a loop, running the model's web_search calls, until
// it answers.
func (a *app) runTurn(cv *conv, t *turn, msgs []dsapi.Message, in aiInput, prompt string) {
	ctx, cancel := context.WithCancel(context.Background())
	cv.cancel = cancel
	t.busy = true
	src, prov, post := a.ai, a.prov, a.post
	live := func() bool { return a.convs[cv.symbol] == cv && ctx.Err() == nil } // on the main thread
	first := msgs == nil
	sources := slices.Clone(cv.sources)
	a.spawn(func() {
		if first {
			in = gather(ctx, prov, in)
			c := buildContext(in)
			sources = slices.Clone(c.Sources)
			msgs = []dsapi.Message{
				{Role: "system", Content: aiSystem(in.Now, in.Sec.Name, in.Symbol)},
				{Role: "user", Content: c.Text + "\n问题：" + prompt},
			}
			post(func() {
				if a.convs[cv.symbol] == cv {
					cv.ctx, cv.sources = &c, slices.Clone(c.Sources)
				}
			})
		}
		searches := 0
		var err error
		for call := 0; call < maxCallsPerTurn; call++ {
			var offer []dsapi.Tool
			if searches < maxSearchesPerTurn && call < maxCallsPerTurn-1 {
				offer = []dsapi.Tool{stockSearchTool}
			}
			post(func() {
				if live() {
					t.answer = ""
				}
			})
			var r dsapi.Reply
			r, err = src.Chat(ctx, msgs, offer, func(s string) {
				post(func() {
					if live() {
						t.answer += s
					}
				})
			})
			if r.Usage != nil {
				u := *r.Usage
				model := src.ModelName()
				post(func() { t.cost += a.billChat(model, u); t.calls++ }) // billed, even when cut short
			}
			if err != nil {
				break
			}
			if len(r.ToolCalls) == 0 {
				msgs = append(msgs, dsapi.Message{Role: "assistant", Content: r.Content})
				break
			}
			msgs = append(msgs, dsapi.Message{Role: "assistant", Content: r.Content, ToolCalls: r.ToolCalls})
			for _, tc := range r.ToolCalls {
				msgs = append(msgs, dsapi.Message{Role: "tool", ToolCallID: tc.ID, Content: a.runTool(ctx, src, post, cv, t, tc, &searches, &sources)})
				if ctx.Err() != nil {
					break
				}
			}
			if ctx.Err() != nil {
				err = ctx.Err()
				break
			}
		}
		post(func() {
			t.busy = false
			if a.convs[cv.symbol] != cv || ctx.Err() != nil {
				return
			}
			cv.cancel = nil
			if err != nil {
				t.err = errText(err)
				return // the conversation goes on without this turn
			}
			cv.msgs = msgs
			a.aiTurns++
		})
	})
}

// runTool runs a web_search of the model's: its results numbered after
// the conversation's sources.
func (a *app) runTool(ctx context.Context, src aiSource, post func(func()), cv *conv, t *turn, tc dsapi.ToolCall, searches *int, sources *[]dsapi.Result) string {
	if tc.Function.Name != stockSearchTool.Function.Name {
		return "没有这个工具：" + tc.Function.Name
	}
	if *searches >= maxSearchesPerTurn {
		return fmt.Sprintf("本轮已搜索 %d 次，达到上限。请用已有信息回答。", maxSearchesPerTurn)
	}
	var args struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil || strings.TrimSpace(args.Query) == "" {
		return "参数无效：需要 {\"query\": \"搜索词\"}"
	}
	*searches++
	q := strings.TrimSpace(args.Query)
	if utf8.RuneCountInString(q) > 128 {
		q = string([]rune(q)[:128])
	}
	n := args.Count
	if n <= 0 {
		n = toolResults
	}
	n = min(n, 10)
	s := &toolSearch{query: q, busy: true}
	post(func() { t.searches = append(t.searches, s) })
	rs, err := src.Search(ctx, q, n)
	if err != nil {
		post(func() { s.busy, s.err = false, errText(err) })
		return "搜索失败：" + errText(err)
	}
	seen := map[string]bool{}
	for _, r := range *sources {
		seen[r.URL] = true
	}
	var fresh []dsapi.Result
	for _, r := range rs {
		if r.URL != "" && !seen[r.URL] {
			seen[r.URL] = true
			fresh = append(fresh, r)
		}
	}
	first := len(*sources) + 1
	*sources = append(*sources, fresh...)
	all := slices.Clone(*sources)
	post(func() {
		s.busy, s.n = false, len(fresh)
		if len(rs) > 0 {
			t.cost += a.billSearch()
		}
		if a.convs[cv.symbol] == cv {
			cv.sources = all
		}
	})
	if len(fresh) == 0 {
		return "没有找到新的结果。"
	}
	return fmt.Sprintf("“%s”的搜索结果（编号 [%d]–[%d]）：\n\n%s", q, first, first+len(fresh)-1, dsapi.Sources(fresh, first))
}

// cite opens source [n] of the conversation, or a link of the answer.
func (a *app) cite(cv *conv, dest string) {
	n := 0
	if _, err := fmt.Sscanf(dest, "cite:%d", &n); err != nil {
		if strings.HasPrefix(dest, "http://") || strings.HasPrefix(dest, "https://") {
			a.open(dest)
		}
		return
	}
	if n >= 1 && n <= len(cv.sources) && cv.sources[n-1].URL != "" {
		a.open(cv.sources[n-1].URL)
	}
}

// citeLinks makes an answer's [n] links to its sources.
func citeLinks(s string, n int) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '[' {
			j := i + 1
			for j < len(s) && j-i <= 3 && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if j > i+1 && j < len(s) && s[j] == ']' && (j+1 >= len(s) || s[j+1] != '(') {
				k, _ := strconv.Atoi(s[i+1 : j])
				if k >= 1 && k <= n {
					fmt.Fprintf(&b, "[[%d]](cite:%d)", k, k)
					i = j
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var citeRef = regexp.MustCompile(`\[(\d{1,3})\]`)

// cited are the sources an answer cites, in order, each once.
func cited(answer string, n int) []int {
	var out []int
	for _, m := range citeRef.FindAllStringSubmatch(answer, -1) {
		k, _ := strconv.Atoi(m[1])
		if k >= 1 && k <= n && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

var keyLeak = regexp.MustCompile(`(?i)[,;]?\s*your api key:\s*\S+`)

// errText is an error to show, with any API key it quotes taken out.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return keyLeak.ReplaceAllString(err.Error(), "")
}

// Billing

// billChat records a completion and returns its estimated cost.
func (a *app) billChat(model string, u dsapi.Usage) float64 {
	c := a.ledger.AddChat(model, billing.Usage{PromptCacheHit: u.PromptCacheHit, PromptCacheMiss: u.PromptCacheMiss, Completion: u.Completion})
	a.billed()
	return c
}

func (a *app) billSearch() float64 {
	a.ledger.AddRequest(billing.Search)
	a.billed()
	return billing.SearchPrice
}

func (a *app) billed() {
	a.sinceBalance++
	if a.sinceBalance >= 20 {
		a.refreshBalance(true)
	}
}

// refreshBalance reads the DeepSeek balance, unless read within a minute.
func (a *app) refreshBalance(force bool) {
	if a.ai == nil || a.balBusy || (!force && a.now().Sub(a.balAt) < time.Minute) {
		return
	}
	a.balBusy, a.sinceBalance = true, 0
	src := a.ai
	a.spawn(func() {
		b, err := src.GetBalance(context.Background())
		a.post(func() {
			a.balBusy, a.balAt = false, a.now()
			if err != nil {
				a.balErr = errText(err)
				return
			}
			a.balance, a.balErr = &b, ""
		})
	})
}

// cny is the balance in yuan (or the first currency), and its symbol.
func (a *app) cny() (float64, string, bool) {
	if a.balance == nil || len(a.balance.Infos) == 0 {
		return 0, "", false
	}
	info := a.balance.Infos[0]
	for _, i := range a.balance.Infos {
		if i.Currency == "CNY" {
			info = i
		}
	}
	v, err := strconv.ParseFloat(info.Total, 64)
	if err != nil {
		return 0, "", false
	}
	sym := "¥"
	if info.Currency == "USD" {
		sym = "$"
	}
	return v, sym, true
}

// yuan formats an estimate: ¥0.12, or ¥0.0046 under a cent.
func yuan(v float64) string {
	switch {
	case v == 0:
		return "¥0"
	case v < 0.01:
		return fmt.Sprintf("¥%.4f", v)
	case v < 100:
		return fmt.Sprintf("¥%.2f", v)
	}
	return fmt.Sprintf("¥%.1f", v)
}

// The DeepSeek key

// saveAIKey checks the key typed with a one-token chat, then keeps it.
func (a *app) saveAIKey() {
	key := strings.TrimSpace(a.editAIKey)
	if key == "" || a.checking {
		return
	}
	a.checking, a.settingsMsg, a.settingsErr = true, "正在验证 DeepSeek API Key…", false
	src, ks, mode := a.newAI(key), a.aiKeys, a.cfg.AIKeyStore
	a.spawn(func() {
		err := src.Check(context.Background())
		var from, warn string
		if err == nil {
			from, warn, err = ks.save(key, mode)
		}
		a.post(func() {
			a.checking = false
			if err != nil {
				a.settingsMsg, a.settingsErr = "验证失败："+errText(err), true
				return
			}
			a.ai, a.aiKey, a.aiKeyFrom, a.aiKeyErr = src, key, from, ""
			a.editAIKey, a.changingAIKey = "", false
			if warn != "" {
				a.cfg.AIKeyStore = "file"
				a.changed(a.cfg)
				a.settingsMsg, a.settingsErr = warn+"，已改存到本地文件", true
			} else {
				a.settingsMsg, a.settingsErr = "已验证并保存到"+aiStoreName(from), false
			}
			a.refreshBalance(true)
		})
	})
}

// testAIKey checks the saved key again, and reads the balance.
func (a *app) testAIKey() {
	if a.ai == nil || a.checking {
		return
	}
	a.checking, a.settingsMsg, a.settingsErr = true, "正在测试…", false
	src := a.ai
	a.spawn(func() {
		err := src.Check(context.Background())
		a.post(func() {
			a.checking = false
			if err != nil {
				a.settingsMsg, a.settingsErr = "测试失败："+errText(err), true
				return
			}
			a.settingsMsg, a.settingsErr = "连接正常：DeepSeek 可用", false
			a.refreshBalance(true)
		})
	})
}

func (a *app) deleteAIKey() {
	if err := a.aiKeys.remove(); err != nil {
		a.settingsMsg, a.settingsErr = "删除失败："+err.Error(), true
		return
	}
	a.ai, a.aiKey, a.aiKeyFrom, a.aiKeyErr, a.balance = nil, "", "", "", nil
	if k := strings.TrimSpace(a.aiKeys.env("DEEPSEEK_API_KEY")); k != "" {
		a.ai, a.aiKey, a.aiKeyFrom = a.newAI(k), k, "env"
	}
	a.settingsMsg, a.settingsErr = "已删除 DeepSeek API Key", false
}

// loadAIKey reads the DeepSeek key at launch; with it, the balance.
func (a *app) loadAIKey() {
	st := a.aiKeys.load(a.cfg.AIKeyStore)
	a.aiKeyErr = st.err
	if st.key != "" {
		a.ai, a.aiKey, a.aiKeyFrom = a.newAI(st.key), st.key, st.from
	}
}

func (a *app) reloadAIKey() {
	st := a.aiKeys.load(a.cfg.AIKeyStore)
	a.aiKeyErr = st.err
	if st.key != "" {
		a.ai, a.aiKey, a.aiKeyFrom = a.newAI(st.key), st.key, st.from
		a.settingsMsg, a.settingsErr = "已读取", false
	} else {
		a.settingsMsg, a.settingsErr = "仍然读不到", true
	}
}

func (a *app) setAIKeyStore(mode string) {
	if mode == a.cfg.AIKeyStore {
		return
	}
	a.cfg.AIKeyStore = mode
	a.changed(a.cfg)
	if a.aiKey == "" || a.aiKeyFrom == "env" {
		return
	}
	from, warn, err := a.aiKeys.save(a.aiKey, mode)
	switch {
	case err != nil:
		a.settingsMsg, a.settingsErr = errText(err), true
	case warn != "":
		a.aiKeyFrom, a.settingsMsg, a.settingsErr = from, warn+"，仍存在本地文件", true
	default:
		a.aiKeyFrom, a.settingsMsg, a.settingsErr = from, "已移到"+aiStoreName(from), false
	}
}
