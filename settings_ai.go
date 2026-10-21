package main

import (
	"fmt"
	"strings"

	"lb-bar/billing"

	"github.com/egoist/mygo/ui"
)

// tabAI is the AI: its provider (DeepSeek, the only one), the API key,
// the balance, what the AI may read, and what it cost.
func (a *app) tabAI(c *ui.Context, k ink) {
	section(c, k, "服务商", "", func() {
		ui.Row(c.Key("provider")).Gap(10).Padding(10, 14).Radius(10).BorderWidth(1).BorderColor(k.line).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "DeepSeek").FontSize(14).FontWeight(600)
				ui.Text(c, "对话与联网搜索 · 模型 "+a.aiModel()+" · 目前唯一支持的服务商").FontSize(11).TextColor(k.muted)
			})
			state, col := "未设置 Key", k.gold
			if a.aiReady() {
				state, col = "已启用", k.green
			}
			ui.Text(c, state).FontSize(11).FontWeight(600).TextColor(col)
		})
	})
	section(c, k, "DeepSeek API Key", "到 platform.deepseek.com 的「API keys」创建，粘贴到这里；保存前会用一次极短的对话验证。", func() {
		if a.aiKeyErr != "" && a.aiKeyFrom != "keychain" {
			ui.Column(c.Key("kcerr")).Gap(8).Padding(10, 12).Radius(10).Background(k.danger.Alpha(0.08)).Children(func() {
				ui.Text(c, a.aiKeyErr+"。点「重试读取」，在弹窗中选「始终允许」；或者改用本地文件保存。").FontSize(12).TextColor(k.danger).LineHeight(1.45)
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c.Key("retry"), "重试读取").OnClick(a.reloadAIKey)
					if a.cfg.AIKeyStore != "file" {
						ui.Button(c.Key("tofile"), "改用本地文件").OnClick(func() { a.setAIKeyStore("file") })
					}
				})
			})
		}
		if a.aiKey != "" && !a.changingAIKey {
			ui.Row(c.Key("saved")).Gap(10).Padding(12, 14).Radius(10).BorderWidth(1).BorderColor(k.line).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Text(c, "已保存  "+mask(a.aiKey)).FontSize(13).FontWeight(550)
					ui.Text(c, "存于："+aiStoreName(a.aiKeyFrom)).FontSize(11).TextColor(k.muted)
				})
				ui.Button(c.Key("test"), "测试").Disabled(a.checking).OnClick(a.testAIKey)
				ui.Button(c.Key("change"), "更换").OnClick(func() { a.changingAIKey, a.editAIKey, a.settingsMsg = true, "", "" })
				if a.aiKeyFrom != "env" {
					ui.Button(c.Key("delete"), "删除").OnClick(a.deleteAIKey)
				}
			})
		} else {
			ui.Row(c.Key("edit")).Gap(8).AlignItems(ui.Center).Children(func() {
				in := ui.TextInput(c.Key("key"), &a.editAIKey).Label("DeepSeek API Key").Placeholder("sk-…").Password().Grow(1).
					Disabled(a.checking).OnSubmit(a.saveAIKey)
				if a.changingAIKey {
					in.AutoFocus()
				}
				ui.PrimaryButton(c.Key("save"), "验证并保存").Disabled(a.checking || strings.TrimSpace(a.editAIKey) == "").OnClick(a.saveAIKey)
				if a.changingAIKey {
					ui.Button(c.Key("cancel"), "取消").OnClick(func() { a.changingAIKey, a.editAIKey = false, "" })
				}
			})
		}
		a.message(c, k)
	})
	if a.aiReady() {
		section(c, k, "余额", "", func() { a.balanceRow(c.Key("bal"), k) })
	}
	section(c, k, "隐私", "AI 只收到你正在看的那只股票的行情、指标、分时和长桥新闻标题。每段对话里都标着附带了什么，点开可看全文。", func() {
		on := a.cfg.AIPositions
		option(c, k, "允许 AI 读取持仓", "默认关闭。开启后，新对话会附带持仓（代码、数量、成本、现价、市值、盈亏），便于问组合问题；从不发送现金、净资产和购买力。", &on, func() {
			a.cfg.AIPositions = on
			a.changed(a.cfg)
		})
		show := !a.cfg.NoAIPane
		option(c, k, "详情里显示 AI 栏", "关闭后详情恢复为两栏；"+cmdKey()+"J 仍可随时打开。", &show, func() {
			a.cfg.NoAIPane = !show
			a.changed(a.cfg)
			if a.expanded {
				a.resize(true)
			}
		})
	})
	section(c, k, "保存位置", "", func() {
		sel := 0
		if a.cfg.AIKeyStore == "file" {
			sel = 1
		}
		ui.Segmented(c.Key("store"), &sel, "macOS 钥匙串（推荐）", "本地文件").Label("保存位置").OnChange(func() {
			mode := ""
			if sel == 1 {
				mode = "file"
			}
			a.setAIKeyStore(mode)
		})
		desc := "和长桥凭证分开保存，钥匙串里是单独一条（账户名 " + aiSecretAccount + "）。"
		if a.cfg.AIKeyStore == "file" {
			desc = "备用方式：保存在 " + a.aiKeys.path() + "（权限 0600），没有系统加密。"
		}
		ui.Text(c, desc).FontSize(11).TextColor(k.muted).LineHeight(1.45)
	})
	ui.Column(c.Key("usage")).Gap(12).Children(func() { a.usageSection(c, k) })
}

// balanceRow is the DeepSeek balance, with 刷新 and the official bill.
func (a *app) balanceRow(c *ui.Context, k ink) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Padding(12, 16).Radius(12).Background(k.card).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, "DeepSeek 账户余额").FontSize(12).TextColor(k.muted)
			switch v, sym, ok := a.cny(); {
			case ok:
				ui.Text(c, fmt.Sprintf("%s%.2f", sym, v)).FontSize(22).FontWeight(650)
			case a.balErr != "":
				ui.Text(c, a.balErr).FontSize(12).TextColor(k.danger)
			case !a.aiReady():
				ui.Text(c, "设置 API Key 后显示").FontSize(13).TextColor(k.muted)
			default:
				ui.Text(c, "读取中…").FontSize(14).TextColor(k.muted)
			}
		})
		if a.balBusy {
			ui.Spinner(c).Size(14, 14)
		}
		ui.Button(c.Key("refresh"), "刷新").Disabled(a.balBusy || !a.aiReady()).OnClick(func() { a.refreshBalance(true) })
		ui.Link(c.Key("official"), "官方账单 ›", "https://platform.deepseek.com/usage").FontSize(12)
	})
}

// usageSection is what the AI cost, estimated: today, this month, in
// all, and the last 30 days.
func (a *app) usageSection(c *ui.Context, k ink) {
	r := a.ledger.Report()
	ui.Text(c, "用量与费用").FontSize(14).FontWeight(650)
	ui.Row(c.Key("stats")).Gap(10).Children(func() {
		stat(c.Key("today"), k, "今日", r.Today)
		stat(c.Key("month"), k, "本月", r.Month)
		stat(c.Key("total"), k, "累计", r.Total)
	})
	usageChart(c.Key("chart"), k, r.Last30)
	ui.Text(c, "每个问题的花费显示在 AI 栏里那条回答的下方。").FontSize(12).TextColor(k.muted)
	ui.Text(c, fmt.Sprintf("本机按官方价格估算：联网搜索每次成功请求 ¥%.3f；AI 对话的每次调用（含工具调用轮次）按 deepseek-flash 实际返回的 token 数计价，区分缓存命中/未命中与北京时间高峰（工作日 9–12、14–18 点）/空闲时段（半价）。以 platform.deepseek.com 的账单为准。", billing.SearchPrice)).
		FontSize(11).TextColor(k.muted).LineHeight(1.5)
	ui.Text(c, "用量账本："+a.dataDir+"/usage.json（只记次数与金额，不记内容）").FontSize(11).TextColor(k.faint).Selectable()
}

func stat(c *ui.Context, k ink, title string, s billing.Summary) {
	ui.Column(c).Grow(1).Basis(0).Gap(6).Padding(12, 14).Radius(12).BorderWidth(1).BorderColor(k.line).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(12).TextColor(k.muted).Grow(1)
			ui.Text(c, "估算").FontSize(10).TextColor(k.faint)
		})
		ui.Text(c, yuan(s.Cost)).FontSize(20).FontWeight(650)
		ui.Text(c, fmt.Sprintf("AI 调用 %d · 搜索 %d", s.Chat, s.Search)).FontSize(11).TextColor(k.muted).SingleLine()
	})
}

func usageChart(c *ui.Context, k ink, days []billing.Day) {
	const h = 80
	top := 0.0
	for _, d := range days {
		top = max(top, d.Cost)
	}
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "近 30 天（估算）").FontSize(13).FontWeight(600).Grow(1)
			if top > 0 {
				ui.Text(c, "最高 "+yuan(top)+" / 天").FontSize(11).TextColor(k.faint)
			}
		})
		ui.Row(c).Height(h).Gap(4).AlignItems(ui.End).Padding(0, 2).BorderWidth(0, 0, 1, 0).BorderColor(k.line).Children(func() {
			for i, d := range days {
				bh := float32(2)
				if top > 0 && d.Cost > 0 {
					bh = max(3, float32(d.Cost/top)*h)
				}
				col := k.link.Alpha(0.75)
				if d.Cost == 0 {
					col = k.line
				}
				if i == len(days)-1 {
					col = k.link
				}
				ui.Box(c.Key(d.Date)).Grow(1).Height(bh).Radius(3, 3, 0, 0).Background(col).
					Tooltip(fmt.Sprintf("%s  %s  AI 调用 %d · 搜索 %d", d.Date[5:], yuan(d.Cost), d.Chat, d.Search))
			}
		})
		ui.Row(c).Children(func() {
			if len(days) > 0 {
				ui.Text(c, days[0].Date[5:]).FontSize(10).TextColor(k.faint).Grow(1)
			}
			ui.Text(c, "今天").FontSize(10).TextColor(k.faint)
		})
	})
}
