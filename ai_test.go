package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lb-bar/dsapi"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// fakeAI is DeepSeek, scripted: each Chat answers the next reply.
type fakeAI struct {
	mu       sync.Mutex
	replies  []dsapi.Reply
	calls    [][]dsapi.Message
	tools    [][]dsapi.Tool
	searches []string
	results  []dsapi.Result
	checkErr error
	checks   int
}

func (f *fakeAI) Chat(_ context.Context, msgs []dsapi.Message, tools []dsapi.Tool, onDelta func(string)) (dsapi.Reply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]dsapi.Message(nil), msgs...))
	f.tools = append(f.tools, tools)
	if len(f.replies) == 0 {
		return dsapi.Reply{}, errors.New("no more replies")
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	if r.Content != "" && onDelta != nil {
		onDelta(r.Content)
	}
	if r.Usage == nil {
		r.Usage = &dsapi.Usage{PromptCacheMiss: 2000, Completion: 300}
	}
	return r, nil
}

func (f *fakeAI) Search(_ context.Context, q string, n int) ([]dsapi.Result, error) {
	f.searches = append(f.searches, q)
	return f.results, nil
}

func (f *fakeAI) GetBalance(context.Context) (dsapi.AccountBalance, error) {
	return dsapi.AccountBalance{Available: true, Infos: []dsapi.Balance{{Currency: "CNY", Total: "48.52"}}}, nil
}

func (f *fakeAI) Check(context.Context) error { f.checks++; return f.checkErr }
func (f *fakeAI) ModelName() string           { return "deepseek-flash" }

func searchCall(id, q string) dsapi.ToolCall {
	tc := dsapi.ToolCall{ID: id, Type: "function"}
	tc.Function.Name = "web_search"
	tc.Function.Arguments = `{"query":"` + q + `"}`
	return tc
}

const exampleAnswer = "腾讯今天高开后震荡上行，午后站稳均价上方 [1]。市场关注即将公布的三季度业绩 [6]。"

func withAI(h *harness) *fakeAI {
	f := &fakeAI{
		replies: []dsapi.Reply{
			{ToolCalls: []dsapi.ToolCall{searchCall("c1", "腾讯控股 今日 股价")}},
			{Content: exampleAnswer},
		},
		results: []dsapi.Result{
			{Title: "腾讯将于11月公布三季度业绩", URL: "https://example.com/tencent-q3", SiteName: "示例财经"},
			{Title: "港股科技股午后走强", URL: "https://example.com/hk-tech", SiteName: "示例财经"},
		},
	}
	h.ai, h.aiKey, h.aiKeyFrom = f, "sk-example-0000", "keychain"
	return f
}

func wideTester(h *harness) *ui.Tester {
	tt := ui.NewTester(h.view, aiExpandedWidth, panelHeight+60)
	tt.Frame()
	return tt
}

func openDetail(t *testing.T, h *harness, tt *ui.Tester, sym string) {
	t.Helper()
	h.query = sym
	h.runSearch()
	h.run()
	h.expand(true)
	h.run()
	tt.Frame()
	if h.detailSym != sym {
		t.Fatalf("detail %q", h.detailSym)
	}
}

func TestContextBuild(t *testing.T) {
	h := newHarness(t)
	tt := h.tester()
	openDetail(t, h, tt, "700.HK")
	in := gather(context.Background(), h.mock, h.aiGather("700.HK"))
	c := buildContext(in)
	for _, want := range []string{"腾讯控股（700.HK", "最新价：", "总市值", "市盈率TTM", "分时（", "长桥资讯最近", "[1] 2026-", "持仓：未附带"} {
		if !strings.Contains(c.Text, want) {
			t.Errorf("context lacks %q:\n%s", want, c.Text)
		}
	}
	if len(c.Sources) == 0 || len(c.Sources) > maxContextNews || c.Sources[0].SiteName != "长桥资讯" || c.Sources[0].Date == nil {
		t.Errorf("sources %+v", c.Sources)
	}
	chip := strings.Join(c.Parts, "、")
	if !strings.HasPrefix(chip, "报价、指标、分时、新闻 ") || strings.Contains(chip, "持仓") {
		t.Errorf("parts %q", chip)
	}
	// Prices and news only: none of the account, ever.
	if strings.Contains(c.Text, "300 股") || strings.Contains(c.Text, "388.2") || strings.Contains(c.Text, "482,315") {
		t.Errorf("context has the account:\n%s", c.Text)
	}
}

// TestPositionsOffByDefault: the holdings are loaded, but the AI is not
// told of them until the user allows it; then of the holdings only.
func TestPositionsOffByDefault(t *testing.T) {
	if defaultConfig().AIPositions {
		t.Fatal("AIPositions is on by default")
	}
	h := newHarness(t)
	f := withAI(h)
	tt := wideTester(h)
	h.key(tt, ui.Cmd, ui.KeyP) // the positions are loaded
	if len(h.positions) != 4 || len(h.balances) != 1 {
		t.Fatalf("positions %d", len(h.positions))
	}
	openDetail(t, h, tt, "AAPL.US")
	h.askAI("我的持仓怎么样？", "")
	h.run()
	sent := f.calls[0][1].Content
	for _, leak := range []string{"贵州茅台", "600519", "388.2", "121.35", "40 股", "482,315", "61,208", "38,120", "持仓（用户允许"} {
		if strings.Contains(sent, leak) {
			t.Errorf("sent %q with positions off:\n%s", leak, sent)
		}
	}
	if !strings.Contains(sent, "持仓：未附带") {
		t.Error("the model was not told the holdings are withheld")
	}
	tt.Frame()
	if !tt.HasText("已附带：报价、指标、分时、新闻 3 条 · 未附带持仓") {
		t.Errorf("chip: %q", tt.Texts())
	}

	h.cfg.AIPositions = true
	h.newChat()
	f.replies = []dsapi.Reply{{Content: "你持有 4 只。"}}
	h.askAI("我的持仓怎么样？", "")
	h.run()
	sent = f.calls[len(f.calls)-1][1].Content
	for _, want := range []string{"持仓（用户允许 AI 读取；共 4 只", "贵州茅台（600519.SH）100 股", "成本 1520.00"} {
		if !strings.Contains(sent, want) {
			t.Errorf("allowed, the context lacks %q:\n%s", want, sent)
		}
	}
	for _, leak := range []string{"482,315", "61,208", "38,120", "120,400", "净资产", "现金 "} {
		if strings.Contains(sent, leak) {
			t.Errorf("sent the account's %q", leak)
		}
	}
	tt.Frame()
	if !tt.HasText("已附带：报价、指标、分时、新闻 3 条、持仓 4 只") {
		t.Errorf("chip: %q", tt.Texts())
	}
}

func TestAskSearchesAndCites(t *testing.T) {
	h := newHarness(t)
	f := withAI(h)
	tt := wideTester(h)
	openDetail(t, h, tt, "700.HK")
	if !tt.HasText(aiDisclaimer) || !tt.HasText("AI 解读 · 腾讯控股") {
		t.Fatalf("no AI pane: %q", tt.Texts())
	}
	h.key(tt, 0, ui.KeyTab) // the detail
	h.key(tt, 0, ui.KeyTab) // the chart
	if !h.chartOn {
		t.Fatal("⇥ did not reach the chart")
	}
	h.key(tt, 0, ui.KeyTab) // the AI pane
	if !h.aiInputH.Focused(nil) && !h.aiOn {
		t.Fatal("⇥ did not reach the AI pane")
	}
	h.typ(tt, "今天为什么涨？")
	h.key(tt, 0, ui.KeyEnter)
	tt.Frame()
	cv := h.chat()
	if cv == nil || len(cv.turns) != 1 {
		t.Fatalf("no turn: %+v", cv)
	}
	tr := cv.turns[0]
	if tr.busy || tr.err != "" || tr.answer != exampleAnswer || tr.calls != 2 {
		t.Fatalf("turn: busy %v err %q calls %d answer %q", tr.busy, tr.err, tr.calls, tr.answer)
	}
	if len(f.searches) != 1 || len(tr.searches) != 1 || tr.searches[0].n != 2 {
		t.Errorf("searches %v %+v", f.searches, tr.searches)
	}
	if len(f.tools[0]) != 1 || f.tools[0][0].Function.Name != "web_search" {
		t.Error("web_search not offered")
	}
	for _, ts := range f.tools { // the model can search, and nothing else: no trading
		for _, tool := range ts {
			if tool.Function.Name != "web_search" {
				t.Errorf("offered %q", tool.Function.Name)
			}
		}
	}
	if got := f.calls[0][0]; got.Role != "system" || !strings.Contains(got.Content, "每轮最多 2 次") {
		t.Errorf("system: %q", got.Content)
	}
	if !strings.Contains(f.calls[0][1].Content, "问题：今天为什么涨？") {
		t.Error("question not sent")
	}
	// The tool's results go on from the news: [6] is the first.
	news := len(cv.ctx.Sources)
	if news == 0 {
		t.Fatal("no news in the context")
	}
	if tool := f.calls[1][len(f.calls[1])-1]; tool.Role != "tool" || !strings.Contains(tool.Content, fmt.Sprintf("[%d]", news+1)) {
		t.Errorf("tool message %q", tool.Content)
	}
	if len(cv.sources) != news+2 || cv.sources[news].URL != "https://example.com/tencent-q3" {
		t.Fatalf("sources %d", len(cv.sources))
	}
	if !tt.HasText("来源") || !tt.HasText("联网搜索：腾讯控股 今日 股价 · 2 条") {
		t.Errorf("view: %q", tt.Texts())
	}
	h.cite(cv, fmt.Sprintf("cite:%d", news+1))
	if len(h.opened) == 0 || h.opened[len(h.opened)-1] != "https://example.com/tencent-q3" {
		t.Errorf("cite opened %v", h.opened)
	}
	// Billed: two completions and a search.
	r := h.ledger.Report()
	if r.Today.Chat != 2 || r.Today.Search != 1 || r.Today.Cost <= 0 || tr.cost != r.Today.Cost {
		t.Errorf("ledger %+v, turn %.5f", r.Today, tr.cost)
	}

	// A follow-up goes on the conversation, with the latest price.
	f.replies = []dsapi.Reply{{Content: "成交额较昨日放大。"}}
	h.typ(tt, "成交量呢？")
	h.key(tt, 0, ui.KeyEnter)
	last := f.calls[len(f.calls)-1]
	if len(last) != 6 || !strings.Contains(last[5].Content, "成交量呢？") || !strings.Contains(last[5].Content, "最新行情：现价") {
		t.Errorf("follow-up: %d messages, %q", len(last), last[len(last)-1].Content)
	}

	// esc and ⇥ go back to the detail; ⌘N starts over.
	h.key(tt, ui.Cmd, ui.KeyN)
	if h.chat() != nil {
		t.Error("⌘N in the AI pane kept the conversation")
	}
	if h.page != pageMain {
		t.Error("⌘N in the AI pane opened the alerts")
	}
	h.key(tt, 0, ui.KeyEscape)
	tt.Frame()
	if h.aiOn || !h.detailOn || !h.expanded {
		t.Errorf("esc: ai %v detail %v expanded %v", h.aiOn, h.detailOn, h.expanded)
	}
}

// TestSearchCap: a third search a turn is refused, and the last call
// offers no tool, so the model answers.
func TestSearchCap(t *testing.T) {
	h := newHarness(t)
	f := withAI(h)
	f.replies = []dsapi.Reply{
		{ToolCalls: []dsapi.ToolCall{searchCall("a", "一"), searchCall("b", "二"), searchCall("c", "三")}},
		{ToolCalls: []dsapi.ToolCall{searchCall("d", "四")}},
		{Content: "好。"},
	}
	tt := wideTester(h)
	openDetail(t, h, tt, "700.HK")
	h.askAI("查", "")
	h.run()
	if len(f.searches) != 2 {
		t.Errorf("searched %d times", len(f.searches))
	}
	if len(f.tools[1]) != 0 {
		t.Error("tools offered past the cap")
	}
	if tr := h.chat().turns[0]; tr.answer != "好。" || tr.err != "" {
		t.Errorf("turn %q %q", tr.answer, tr.err)
	}
}

func TestQuickAction(t *testing.T) {
	h := newHarness(t)
	f := withAI(h)
	h.cfg.NoAIPane = true
	tt := wideTester(h)
	h.key(tt, 0, ui.KeyDown)
	sym := h.chosenSym()
	h.key(tt, ui.Cmd, ui.KeyJ)
	tt.Frame()
	if !h.expanded || h.detailSym != sym || h.cfg.NoAIPane {
		t.Fatalf("⌘J: expanded %v detail %q pane %v", h.expanded, h.detailSym, !h.cfg.NoAIPane)
	}
	cv := h.chat()
	if cv == nil || cv.turns[0].question != todayLabel || !strings.Contains(f.calls[0][1].Content, "问题："+todayPrompt) {
		t.Fatalf("quick action: %+v", cv)
	}
	if !tt.HasText(todayLabel) || !tt.HasText("已附带：") {
		t.Errorf("view: %q", tt.Texts())
	}
	// And the button, on another security.
	openDetail(t, h, tt, "AAPL.US")
	if !tt.HasText("✦ 解读今日走势") {
		t.Fatalf("no button: %q", tt.Texts())
	}
	f.replies = []dsapi.Reply{{Content: "苹果今天平开。"}}
	if err := tt.Click("✦ " + todayLabel + "  " + cmdKey() + "J"); err != nil {
		t.Fatal(err)
	}
	h.run()
	tt.Frame()
	if cv := h.chat(); cv == nil || cv.symbol != "AAPL.US" || cv.turns[0].answer != "苹果今天平开。" {
		t.Errorf("button: %+v", cv)
	}
}

func TestAIWithoutKey(t *testing.T) {
	h := newHarness(t)
	tt := wideTester(h)
	openDetail(t, h, tt, "700.HK")
	if !tt.HasText("去设置") || !tt.HasText(aiDisclaimer) {
		t.Fatalf("no setup: %q", tt.Texts())
	}
	if h.askAI("hi", "") {
		t.Error("asked without a key")
	}
	if err := tt.Click("去设置"); err != nil {
		t.Fatal(err)
	}
	h.run()
	if h.page != pageSettings || h.tab != tabAI {
		t.Errorf("page %d tab %d", h.page, h.tab)
	}
}

func TestAIKeySettings(t *testing.T) {
	h := newHarness(t)
	f := &fakeAI{}
	h.newAI = func(string) aiSource { return f }
	mem := &memSecrets{}
	h.aiKeys.secrets = mem
	tt := wideTester(h)
	h.openTab(tabAI)
	tt.Frame()
	if !tt.HasText("允许 AI 读取持仓") || h.cfg.AIPositions {
		t.Fatalf("tab: %q", tt.Texts())
	}
	h.editAIKey = "sk-example-1234567890"
	h.saveAIKey()
	h.run()
	if h.aiKey != "sk-example-1234567890" || h.aiKeyFrom != "keychain" || mem.m["|"+aiSecretAccount] != h.aiKey || f.checks != 1 {
		t.Fatalf("saved: from %q checks %d", h.aiKeyFrom, f.checks)
	}
	tt.Frame()
	if tt.HasText("sk-example-1234567890") {
		t.Error("the key is shown")
	}
	if !tt.HasText("¥48.52") {
		t.Errorf("no balance: %q", tt.Texts())
	}
	// The keychain refuses: the file, 0600.
	mem.fail = errDenied
	h.setAIKeyStore("file")
	st, err := os.Stat(filepath.Join(h.dataDir, aiKeyFile))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v %v", err, st)
	}
	got := h.aiKeys.load("file")
	if got.key != "sk-example-1234567890" || got.from != "file" {
		t.Errorf("load %+v", got)
	}
	// The env wins when nothing is saved.
	env := aiKeyStore{secrets: &memSecrets{}, dir: t.TempDir(), env: func(k string) string {
		if k == "DEEPSEEK_API_KEY" {
			return "sk-env"
		}
		return ""
	}}
	if st := env.load(""); st.key != "sk-env" || st.from != "env" {
		t.Errorf("env %+v", st)
	}
	// A wrong key is not saved.
	f.checkErr = errors.New("401 Authentication Fails, Your api key: sk-bad**** is invalid")
	h.changingAIKey, h.editAIKey = true, "sk-bad"
	h.saveAIKey()
	h.run()
	if h.aiKey != "sk-example-1234567890" || !h.settingsErr || strings.Contains(h.settingsMsg, "sk-bad") {
		t.Errorf("bad key: %q %q", h.aiKey, h.settingsMsg)
	}
}

func TestUsageTab(t *testing.T) {
	h := newHarness(t)
	withAI(h)
	tt := wideTester(h)
	openDetail(t, h, tt, "700.HK")
	h.askAI("q", "")
	h.run()
	h.openTab(tabAI)
	tt.Frame()
	for _, want := range []string{"今日", "本月", "累计", "近 30 天（估算）", "AI 调用 2 · 搜索 1"} {
		if !tt.HasText(want) {
			t.Errorf("usage lacks %q: %q", want, tt.Texts())
		}
	}
}

func TestErrTextHidesKey(t *testing.T) {
	got := errText(errors.New("401: Authentication Fails, Your api key: sk-abc123 is invalid"))
	if strings.Contains(got, "sk-abc123") {
		t.Errorf("%q", got)
	}
}

func TestCitedAndLinks(t *testing.T) {
	if got := citeLinks("涨了 [1][3]，见 [9]。", 3); got != "涨了 [[1]](cite:1)[[3]](cite:3)，见 [9]。" {
		t.Errorf("%q", got)
	}
	if got := cited("a [2] b [1] c [2] [8]", 5); len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Errorf("%v", got)
	}
}

var _ = time.Second
var _ = feed.Shanghai
