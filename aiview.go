package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

var iconSparkles = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.94 14.06 8 20l-1.94-5.94L0 12l6.06-1.94L8 4l1.94 6.06L16 12z" transform="translate(2 -1) scale(.9)"/><path d="M20 3v4M22 5h-4"/></svg>`))

const aiDisclaimer = "AI 生成内容仅供参考，不构成投资建议"

// aiPaneView is the AI conversation about the detail's security, on the
// right of the detail: the context sent, the turns, the question box,
// and the disclaimer.
func (a *app) aiPaneView(c *ui.Context, k ink) {
	cv := a.chat()
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		ui.Row(c.Key("head")).Padding(12, 14, 8, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconSparkles).FontSize(14).TextColor(k.link)
			title := "AI 解读"
			if a.detailSym != "" {
				title += " · " + shortName(a.name(a.detailSym), a.detailSym)
			}
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(k.link).SingleLine().Shrink(1)
			if cv != nil && cv.busy() {
				ui.Spinner(c).Size(11, 11)
			}
			ui.Spacer(c)
			if cv != nil && len(cv.turns) > 0 {
				ui.Row(c.Key("new")).Padding(3, 8).Radius(6).Cursor(ui.CursorPointer).Label("新对话").
					Tooltip("清空这只股票的对话  " + cmdKey() + "N").OnClick(a.newChat).Children(func() {
					ui.Text(c, "新对话 "+cmdKey()+"N").FontSize(11).TextColor(k.muted)
				})
			}
		})
		a.contextChip(c.Key("ctx"), k, cv)
		ui.Divider(c)
		switch {
		case !a.aiReady():
			a.aiSetup(c.Key("setup"), k)
		case cv == nil || len(cv.turns) == 0:
			a.aiEmpty(c.Key("empty"), k)
		default:
			ui.List(c.Key("turns"), &a.aiList, len(cv.turns), func(i int) {
				a.turnView(c.Key(i), k, cv, i)
			}).Grow(1).MinHeight(0).Padding(12, 16, 12, 16).Gap(18)
		}
		ui.Divider(c)
		ui.Column(c.Key("composer")).Padding(8, 12, 6, 14).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				busy := cv != nil && cv.busy()
				ph := "问问这只股票…（↵ 发送）"
				if !a.aiReady() {
					ph = "设置 DeepSeek API Key 后可提问"
				}
				ui.TextAreaBase(c.Key("draft"), &a.aiDraft).Bind(&a.aiInputH).Grow(1).FontSize(13).MaxHeight(84).
					Placeholder(ph).Label("向 AI 提问").Disabled(!a.aiReady()).OnTextKey(a.aiDraftKey)
				if a.focusAI {
					a.focusAI = false
					a.aiInputH.Focus()
				}
				if cv != nil && len(cv.turns) > 0 {
					ui.Button(c.Key("today"), "解读今日").Disabled(busy || !a.aiReady()).Tooltip(todayLabel + "  " + cmdKey() + "J").OnClick(a.quickAction)
				}
			})
			ui.Text(c, aiDisclaimer).FontSize(10).TextColor(k.faint).SingleLine()
		})
	})
	a.aiOn = a.aiInputH.Focused(c)
}

// contextChip says what the conversation sent: 已附带：报价、指标、新闻 5 条.
// A click shows it all.
func (a *app) contextChip(c *ui.Context, k ink, cv *conv) {
	ui.Column(c).Padding(0, 14, 8, 16).Gap(6).Children(func() {
		var text string
		switch {
		case cv != nil && cv.ctx != nil:
			text = "已附带：" + strings.Join(cv.ctx.Parts, "、")
			if !a.cfg.AIPositions {
				text += " · 未附带持仓"
			}
		case cv != nil && cv.busy():
			text = "正在整理行情上下文…"
		default:
			parts := "报价、指标、分时、新闻"
			if a.cfg.AIPositions {
				parts += "、持仓"
			}
			text = "提问时将附带：" + parts
			if !a.cfg.AIPositions {
				text += "（不含持仓）"
			}
		}
		chip := ui.Row(c.Key("chip")).Padding(3, 10).Radius(20).BorderWidth(1).BorderColor(k.line).AlignItems(ui.Center).Gap(6)
		if cv != nil && cv.ctx != nil {
			chip.Cursor(ui.CursorPointer).Tooltip("点击查看发送给 AI 的全部内容").Label(text).OnClick(func() { a.showCtx = !a.showCtx })
		}
		chip.Children(func() {
			ui.Text(c, text).FontSize(11).TextColor(k.muted).SingleLine().Shrink(1)
			if cv != nil && cv.ctx != nil {
				arrow := "›"
				if a.showCtx {
					arrow = "⌄"
				}
				ui.Text(c, arrow).FontSize(11).TextColor(k.faint)
			}
		})
		if a.showCtx && cv != nil && cv.ctx != nil {
			ui.Scroll(c.Key("full")).MaxHeight(160).Padding(8, 10).Radius(8).Background(k.card).Children(func() {
				ui.Text(c, cv.ctx.Text).FontSize(11).TextColor(k.muted).LineHeight(1.45).Selectable()
			})
		}
	})
}

func (a *app) aiSetup(c *ui.Context, k ink) {
	ui.Column(c).Grow(1).Padding(36, 24).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, "AI 解读需要 DeepSeek API Key").FontSize(14).FontWeight(600)
		ui.Text(c, "到 platform.deepseek.com 创建 Key，粘贴到 设置 › DeepSeek。按次计费，费用在 设置 › 用量与费用 查看。").
			FontSize(12).TextColor(k.muted).TextAlign(ui.Center).LineHeight(1.5)
		ui.PrimaryButton(c.Key("go"), "去设置").OnClick(func() { a.openTab(tabDeepSeek) })
	})
}

// aiEmpty is a conversation not started: the quick action, and a few
// questions to ask.
func (a *app) aiEmpty(c *ui.Context, k ink) {
	ui.Column(c).Grow(1).Padding(28, 20).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.PrimaryButton(c.Key("today"), "✦ "+todayLabel+"  "+cmdKey()+"J").Disabled(a.detailSym == "").OnClick(a.quickAction)
		ui.Text(c, "或直接提问，例如：").FontSize(12).TextColor(k.faint)
		for _, q := range []string{"最近一个季度业绩怎么样？", "今天为什么涨跌？", "估值在同行里算高吗？"} {
			ui.Row(c.Key(q)).Padding(5, 12).Radius(16).BorderWidth(1).BorderColor(k.line).Cursor(ui.CursorPointer).Label(q).
				OnClick(func() { a.askAI(q, "") }).Children(func() {
				ui.Text(c, q).FontSize(12).TextColor(k.muted)
			})
		}
	})
}

// aiDraftKey sends with ↵, ⇧↵ breaks the line; ↑ in an empty box brings
// the last question back.
func (a *app) aiDraftKey(t ui.TextKey) bool {
	switch {
	case t.Key == ui.KeyEnter && t.Mods == 0:
		if a.askAI(t.Text(), "") {
			t.SetText("")
		}
		return true
	case (t.Key == ui.KeyTab || t.Key == ui.KeyEscape) && t.Mods == 0:
		a.focusDetail = true
		return true
	case t.Key == ui.KeyTab && t.Mods == ui.Shift:
		a.focusList = true
		return true
	case t.Key == ui.KeyUp && t.Mods == 0 && t.Text() == "" && a.lastQuestion != "":
		t.SetText(a.lastQuestion)
		return true
	}
	return false
}

// turnView is a turn: the question, its searches, the answer, its sources.
func (a *app) turnView(c *ui.Context, k ink, cv *conv, i int) {
	t := cv.turns[i]
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c.Key("q")).Justify(ui.End).Children(func() {
			ui.Text(c, t.question).FontSize(13).Padding(6, 11).Radius(12).Background(k.chosen).MaxWidth(360)
		})
		for j, s := range t.searches {
			text := "联网搜索：" + s.query
			col := k.muted
			switch {
			case s.busy:
				text = "正在搜索：" + s.query + "…"
			case s.err != "":
				text, col = "搜索失败："+s.query+"（"+s.err+"）", k.danger
			default:
				text += fmt.Sprintf(" · %d 条", s.n)
			}
			ui.Row(c.Key(fmt.Sprint("s", j))).Gap(6).AlignItems(ui.Center).Children(func() {
				if s.busy {
					ui.Spinner(c).Size(11, 11)
				} else {
					ui.Icon(c, iconSearch).FontSize(12).TextColor(col)
				}
				ui.Text(c, text).FontSize(11).TextColor(col).SingleLine().Shrink(1)
			})
		}
		switch {
		case t.err != "":
			ui.Text(c.Key("err"), t.err).FontSize(12).TextColor(k.danger)
		case t.answer == "" && t.busy && !searchBusy(t):
			ui.Row(c.Key("wait")).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(11, 11)
				label := "思考中…"
				if cv.ctx == nil {
					label = "整理行情、指标和新闻…"
				}
				ui.Text(c, label).FontSize(12).TextColor(k.faint)
			})
		}
		if t.answer != "" {
			src := citeLinks(t.answer, len(cv.sources))
			t.md.Theme = &markdown.Theme{FontSize: 13, Link: k.link}
			if t.busy {
				markdown.Stream(c.Key("md"), src, &t.md)
			} else {
				markdown.View(c.Key("md"), src, &t.md)
			}
		}
		if !t.busy && t.answer != "" {
			if ns := cited(t.answer, len(cv.sources)); len(ns) > 0 {
				ui.Column(c.Key("src")).Gap(2).Padding(6, 0, 0, 0).Children(func() {
					ui.Text(c, "来源").FontSize(10).FontWeight(600).TextColor(k.faint)
					for _, n := range ns {
						r := cv.sources[n-1]
						site := r.SiteName
						if site == "" {
							site = hostOf(r.URL)
						}
						label := fmt.Sprintf("[%d] %s · %s", n, r.Title, site)
						ui.Link(c.Key(n), label, r.URL).FontSize(11).SingleLine()
					}
				})
			}
			if t.calls > 0 {
				line := fmt.Sprintf("%s · %d 次调用", a.aiModel(), t.calls)
				if len(t.searches) > 0 {
					line += fmt.Sprintf(" · 搜索 %d 次", len(t.searches))
				}
				line += " · 约 " + yuan(t.cost)
				ui.Text(c.Key("cost"), line).FontSize(10).TextColor(k.faint)
			}
		}
	})
}

func searchBusy(t *turn) bool {
	for _, s := range t.searches {
		if s.busy {
			return true
		}
	}
	return false
}

func (a *app) aiModel() string {
	if a.ai == nil {
		return "deepseek-flash"
	}
	return a.ai.ModelName()
}

func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	return strings.TrimPrefix(u, "www.")
}
