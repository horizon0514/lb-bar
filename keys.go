package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

// macHints spells keys as macOS does (⌘, ⌥), on macOS; tests set it to
// render the Mac's hints elsewhere.
var macHints = runtime.GOOS == "darwin"

func cmdKey() string {
	if macHints {
		return "⌘"
	}
	return "Ctrl+"
}

// keys spells a shortcut as the system does: ⌥S on macOS, Alt+Shift+S
// elsewhere.
func keys(shortcut string) string {
	if !macHints {
		return shortcut
	}
	return macKeys(shortcut)
}

// macKeys spells a shortcut as macOS does, everywhere.
func macKeys(shortcut string) string {
	return strings.NewReplacer("CmdOrCtrl+", "⌘", "Cmd+", "⌘", "Ctrl+", "⌃", "Alt+", "⌥", "Option+", "⌥", "Shift+", "⇧", "Space", "空格").Replace(shortcut)
}

var digitKeys = []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}

// keysMain: the keyboard on the main page, as in Spotlight. The list has
// it when the panel shows: ↑↓ choose, ↵ the detail, ⌘↵ the quote page;
// typing goes to the search field (listInput). ⇥ moves between the list
// and the detail; esc closes the detail, then clears the search, then
// hides.
func (a *app) keysMain(c *ui.Context) {
	onInput := a.input.Focused(c)
	onDetail := a.expanded && a.detailFocus.FocusWithin(c) // the chart too
	if c.Shortcut(ui.Cmd, ui.KeyJ) {
		a.quickAction()
		return
	}
	if a.expanded && a.aiPane() && a.aiInputH.Focused(c) {
		a.keysAI(c)
		return
	}
	switch {
	case c.Shortcut(0, ui.KeyEscape):
		switch {
		case onDetail:
			a.focusList = true
		case a.expanded:
			a.expand(false)
		case a.searching():
			a.query = ""
			a.runSearch()
			a.focusList = true
		default:
			a.hide()
		}
		return
	case c.Shortcut(ui.Cmd, ui.KeyL), c.Shortcut(ui.Cmd, ui.KeyF):
		a.focusQuery = true
		a.input.Focus()
	case c.Shortcut(ui.Alt, ui.KeyLeft), c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft), c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab):
		a.setGroup(a.groupIdx - 1)
	case c.Shortcut(ui.Alt, ui.KeyRight), c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight), c.Shortcut(ui.Ctrl, ui.KeyTab):
		a.setGroup(a.groupIdx + 1)
	case c.Shortcut(ui.Cmd, ui.KeyP), c.Shortcut(ui.Cmd, ui.Key0):
		a.setGroup(len(a.groups))
	case c.Shortcut(ui.Cmd, ui.KeyR):
		a.refreshQuotes(true)
		a.refreshMarkets()
		if a.showingPositions() {
			a.loadPositions(true)
		}
		a.toast = "已刷新"
	case c.Shortcut(ui.Cmd, ui.KeyEqual) && a.expanded && a.period > 0:
		a.zoom(1)
	case c.Shortcut(ui.Cmd, ui.KeyMinus) && a.expanded && a.period > 0:
		a.zoom(-1)
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyC) && a.showingPositions():
		a.cyclePosCurrency()
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyH):
		a.cfg.HideAmounts = !a.cfg.HideAmounts
		a.changed(a.cfg)
	case c.Shortcut(ui.Cmd, ui.KeyEnter):
		sym := a.chosenSym()
		if onDetail {
			sym = a.detailSym
		}
		a.openWeb(sym)
	case c.Shortcut(ui.Cmd, ui.KeyD):
		a.togglePin()
	case c.Shortcut(ui.Cmd, ui.KeyN):
		a.newAlert()
		return
	}
	for i, key := range digitKeys {
		if c.Shortcut(ui.Cmd, key) && i <= len(a.groups) {
			a.setGroup(i)
		}
	}
	if onDetail {
		a.keysDetail(c)
		return
	}
	if c.Shortcut(0, ui.KeyTab) {
		if a.chosenSym() != "" {
			a.expand(true)
			a.focusDetail = true
		}
		return
	}
	n := len(a.rows())
	if onInput {
		switch {
		case c.Shortcut(0, ui.KeyDown) && n > 0:
			a.listFocus.Focus()
			a.choose(max(a.chosen, 0))
		case c.Shortcut(0, ui.KeyUp) && n > 0:
			a.choose(a.chosen - 1)
		}
		return
	}
	switch {
	case c.Shortcut(0, ui.KeyDown):
		a.choose(a.chosen + 1)
	case c.Shortcut(0, ui.KeyUp):
		if a.chosen <= 0 {
			a.input.Focus()
		} else {
			a.choose(a.chosen - 1)
		}
	case c.Shortcut(ui.Cmd, ui.KeyDown), c.Shortcut(0, ui.KeyEnd):
		a.choose(n - 1)
	case c.Shortcut(ui.Cmd, ui.KeyUp), c.Shortcut(0, ui.KeyHome):
		a.choose(0)
	case c.Shortcut(0, ui.KeyPageDown):
		a.choose(a.chosen + 8)
	case c.Shortcut(0, ui.KeyPageUp):
		a.choose(a.chosen - 8)
	case c.Shortcut(0, ui.KeyEnter):
		a.expand(true)
	case c.Shortcut(0, ui.KeyRight) && a.expanded:
		a.setPeriod(a.period + 1)
	case c.Shortcut(0, ui.KeyLeft) && a.expanded:
		a.setPeriod(a.period - 1)
	case c.Shortcut(ui.Cmd, ui.KeyC):
		if s := a.chosenSym(); s != "" {
			a.copy(s)
			a.toast = "已复制 " + s
		}
	}
}

// keysDetail: on the detail pane, ←→ change the chart, ↑↓ choose a news
// item, ↵ opens it; ⇥ goes to the chart (chartKeys), and from there to
// the AI pane; ⇧⇥ back to the list.
func (a *app) keysDetail(c *ui.Context) {
	ns := a.news[a.detailSym]
	switch {
	case c.Shortcut(0, ui.KeyTab):
		a.focusChart = true
	case c.Shortcut(ui.Shift, ui.KeyTab):
		a.focusList = true
	case c.Shortcut(0, ui.KeyRight):
		a.setPeriod(a.period + 1)
	case c.Shortcut(0, ui.KeyLeft):
		a.setPeriod(a.period - 1)
	case c.Shortcut(0, ui.KeyDown):
		a.newsChosen = min(a.newsChosen+1, min(len(ns), maxNews)-1)
	case c.Shortcut(0, ui.KeyUp):
		a.newsChosen = max(a.newsChosen-1, -1)
	case c.Shortcut(0, ui.KeyEnter):
		if a.newsChosen >= 0 && a.newsChosen < len(ns) {
			a.open(ns[a.newsChosen].URL)
		} else {
			a.openWeb(a.detailSym)
		}
	}
}

// keysAI: in the AI pane's question box, ↵ sends (aiDraftKey); ⇥ and
// esc go back to the detail, ⇧⇥ to the list; ⌘N starts over.
func (a *app) keysAI(c *ui.Context) {
	switch {
	case c.Shortcut(0, ui.KeyEscape), c.Shortcut(0, ui.KeyTab):
		a.focusDetail = true
	case c.Shortcut(ui.Shift, ui.KeyTab):
		a.focusChart = true
	case c.Shortcut(ui.Cmd, ui.KeyN):
		a.newChat()
	case c.Shortcut(ui.Cmd, ui.KeyEnter):
		a.openWeb(a.detailSym)
	}
}

// listInput takes the typing on the list to the search field: a
// character, or one composed with an input method, adds to it; ⌫ takes
// its last off.
func (a *app) listInput(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputCompose:
		a.composing = ev.Text
		return true
	case ui.InputText:
		a.composing = ""
		if ev.Text == "" || ev.Text == " " || ev.Text == "\t" || ev.Text == "\r" || ev.Text == "\n" {
			return false
		}
		a.query += ev.Text
	case ui.InputKeyDown:
		if ev.Key != ui.KeyBackspace || ev.Mods != 0 || a.query == "" {
			return false
		}
		r := []rune(a.query)
		a.query = string(r[:len(r)-1])
	default:
		return false
	}
	a.caretToEnd = true
	a.input.Focus()
	a.runSearch()
	return true
}

// shortcutGroups is the cheatsheet (⌘/), and the README's table.
func shortcutGroups() [][3]string {
	gs := macShortcuts()
	for i := range gs {
		gs[i][1], gs[i][2] = hk(gs[i][1]), hk(gs[i][2])
	}
	return gs
}

// macShortcuts are the shortcuts, spelled as on the Mac; hk spells them
// for the others.
func macShortcuts() [][3]string {
	m := "⌘"
	return [][3]string{
		{"列表", "↑ ↓", "选择"},
		{"", "↵", "看详情（分时 / K 线 / 新闻）"},
		{"", m + "↵", "在长桥网页打开"},
		{"", "直接打字", "搜索：代码、中英文名称，如 700、AAPL、茅台、tencent"},
		{"", "⌥←  ⌥→  /  ⌃⇥", "切换自选分组"},
		{"", m + "1 … " + m + "9", "第 N 个分组"},
		{"", m + "P", "持仓（只读）"},
		{"", m + "D", "置顶到菜单（常用股票）/ 取消"},
		{"", m + "N", "为选中的股票加价格提醒（在 AI 栏里是新对话）"},
		{"", m + "C", "复制代码"},
		{"详情", "⇥", "列表 → 详情 → 图表 → AI 栏 之间切换（⇧⇥ 往回）"},
		{"", "←  →", "切换 分时 / 日K / 周K / 月K"},
		{"", "↑ ↓  ↵", "选新闻 / 打开"},
		{"", m + "=  " + m + "-", "K 线放大 / 缩小（30 / 60 / 120 / 250 根）"},
		{"", "esc", "收起详情"},
		{"图表", "←  →", "十字光标逐根移动（到最左继续往前翻）"},
		{"", "Home  End", "第一根 / 最后一根"},
		{"", "+  −", "K 线放大 / 缩小"},
		{"", "esc", "清除十字光标"},
		{"持仓", "⇧" + m + "C", "合计币种 HKD → USD → CNY"},
		{"AI", m + "J", "解读今日走势（一键，不用打字）"},
		{"", "↵  ⇧↵", "发送问题 / 换行"},
		{"", m + "N", "在 AI 栏：新对话"},
		{"", "esc", "从 AI 栏回到详情"},
		{"通用", macKeys(defaultShortcut), "唤起 / 隐藏面板（可在设置里改）"},
		{"", m + "L", "回到搜索框"},
		{"", m + "R", "刷新"},
		{"", "⇧" + m + "H", "隐藏 / 显示金额"},
		{"", "esc", "清空搜索；再按隐藏面板"},
		{"", m + ",", "设置（" + m + "1–6：通用、账号、AI、提醒、快捷键、关于）"},
		{"", "拖动顶栏 / 底栏", "移动面板（双击复位）"},
		{"", m + "/", "本快捷键表"},
		{"", m + "Q", "退出 LB Bar"},
	}
}

// hint is the hint bar's text for a list of keys and what they do.
func hint(pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		if b.Len() > 0 {
			b.WriteString("   ")
		}
		b.WriteString(hk(pairs[i]) + " " + pairs[i+1])
	}
	return b.String()
}
