package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"lb-bar/alerts"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// settings is the settings page: tabs on the left (⌘1–⌘5), the tab's
// sections on the right.
func (a *app) settings(c *ui.Context, k ink) {
	for i, key := range digitKeys[:len(tabNames)] {
		if c.Shortcut(ui.Cmd, key) {
			a.openTab(i)
		}
	}
	ui.Column(c).Fill().Children(func() {
		a.grip(ui.Row(c.Key("head"))).Padding(12, 16, 12, 12).Gap(10).AlignItems(ui.Center).Children(func() {
			if a.configured() {
				ui.Row(c.Key("back")).Gap(2).Padding(4, 8, 4, 4).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
					Label("返回").OnClick(a.back).Children(func() {
					ui.Icon(c, iconBack).FontSize(16).TextColor(k.muted)
					ui.Text(c, "返回").FontSize(13).TextColor(k.muted)
				})
			}
			ui.Text(c, "设置").FontSize(15).FontWeight(650).Grow(1)
			ui.Text(c, "LB Bar "+version).FontSize(11).TextColor(k.faint)
		})
		ui.Divider(c)
		ui.Row(c.Key("body")).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c.Key("tabs")).Width(132).Padding(10, 8).Gap(2).Background(k.card).Children(func() {
				for i, name := range tabNames {
					on := i == a.tab
					bg, fg := ui.Transparent, k.text
					if on {
						bg, fg = k.chosen, k.link
					}
					ui.Row(c.Key(name)).Padding(7, 10).Radius(7).Background(bg).AlignItems(ui.Center).Cursor(ui.CursorPointer).
						Focusable().Label(name).OnClick(func() { a.openTab(i) }).Children(func() {
						w := 450
						if on {
							w = 600
						}
						ui.Text(c, name).FontSize(13).FontWeight(w).TextColor(fg).Grow(1).SingleLine()
						ui.Text(c, cmdKey()+fmt.Sprint(i+1)).FontSize(10).TextColor(k.faint)
					})
				}
			})
			ui.Divider(c)
			ui.Scroll(c.Key(fmt.Sprint("tab", a.tab))).Grow(1).MinHeight(0).Padding(18, 24, 24, 24).Gap(20).Children(func() {
				switch a.tab {
				case tabGeneral:
					a.tabGeneral(c, k)
				case tabAccount:
					a.tabAccount(c, k)
				case tabAI:
					a.tabAI(c, k)
				case tabAlerts:
					a.tabAlerts(c, k)
				case tabKeys:
					a.tabKeys(c, k)
				case tabAbout:
					a.tabAbout(c, k)
				}
			})
		})
		ui.Divider(c)
		esc := "返回"
		if !a.configured() {
			esc = "隐藏"
		}
		left := ""
		if a.tab == tabAI {
			left = "金额均为估算"
		}
		a.hintBar(c, k, left, hint(cmdKey()+"1–"+fmt.Sprint(len(tabNames)), "切换", "⇥", "下一项", "↵", "确定", "esc", esc))
	})
}

// section is a titled group of settings.
func section(c *ui.Context, k ink, title, desc string, body func()) {
	ui.Column(c.Key(title)).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(600)
		if desc != "" {
			ui.Text(c, desc).FontSize(12).TextColor(k.muted).LineHeight(1.45)
		}
		body()
	})
}

// option is a row with a switch on its right.
func option(c *ui.Context, k ink, label, desc string, on *bool, changed func()) {
	ui.Row(c.Key(label)).Gap(12).Padding(9, 14).Radius(10).BorderWidth(1).BorderColor(k.line).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(13)
			if desc != "" {
				ui.Text(c, desc).FontSize(11).TextColor(k.muted).LineHeight(1.4)
			}
		})
		if ui.Switch(c.Key("sw"), on).Label(label).Changed() {
			changed()
		}
	})
}

func (a *app) message(c *ui.Context, k ink) {
	if a.settingsMsg == "" {
		return
	}
	col := k.green
	if a.settingsErr {
		col = k.danger
	}
	ui.Row(c.Key("msg")).Gap(8).AlignItems(ui.Center).Children(func() {
		if a.checking {
			ui.Spinner(c).Size(12, 12)
		}
		ui.Text(c, a.settingsMsg).FontSize(12).TextColor(col).Grow(1).LineHeight(1.4)
	})
}

func (a *app) tabGeneral(c *ui.Context, k ink) {
	section(c, k, plat.bar, barDesc(), func() {
		if !plat.title {
			ui.Text(c, "悬停提示现在显示："+strings.ReplaceAll(a.trayTip(), "\n", " · ")).FontSize(11).TextColor(k.muted).LineHeight(1.45)
			return
		}
		sel := 0
		if a.cfg.Menubar == "icon" {
			sel = 1
		}
		ui.Segmented(c.Key("menubar"), &sel, "显示行情", "只显示图标").Label(plat.bar + "显示").OnChange(func() {
			a.cfg.Menubar = []string{"ticker", "icon"}[sel]
			a.changed(a.cfg)
			a.trayChanged()
		})
		if a.cfg.Menubar != "icon" {
			rotate := a.cfg.Rotate
			option(c, k, "轮播", fmt.Sprintf("%s文字在常用股票之间每 %d 秒换一只；关闭则只显示第一只。", plat.bar, a.cfg.RotateSec), &rotate, func() {
				a.cfg.Rotate = rotate
				a.changed(a.cfg)
				a.trayChanged()
			})
			code := a.cfg.MenubarCode
			option(c, k, "显示代码而不是名称", "例如 700 512.50 +1.20%", &code, func() {
				a.cfg.MenubarCode = code
				a.changed(a.cfg)
				a.trayChanged()
			})
			dot := a.cfg.Dot
			option(c, k, "彩色圆点", "在前面加 🔴 / 🟢 表示涨跌，颜色跟随下面的涨跌颜色。", &dot, func() {
				a.cfg.Dot = dot
				a.changed(a.cfg)
				a.trayChanged()
			})
		}
		ui.Text(c, "现在显示："+orDash(a.trayTitle())).FontSize(11).TextColor(k.muted)
	})
	section(c, k, "常用股票", pinsDesc(), func() {
		a.tickerList(c.Key("tickers"), k)
	})
	section(c, k, "涨跌颜色", "", func() {
		sel := 0
		if a.cfg.GreenUp {
			sel = 1
		}
		ui.Segmented(c.Key("colors"), &sel, "红涨绿跌", "绿涨红跌").Label("涨跌颜色").OnChange(func() {
			a.cfg.GreenUp = sel == 1
			a.changed(a.cfg)
			a.trayChanged()
		})
	})
	section(c, k, "唤起快捷键", "全局快捷键，任何时候都能唤起面板。被占用时换一个，例如 Alt+Shift+S、Cmd+Shift+K。", func() {
		a.shortcutField(c, k)
	})
	section(c, k, "连接", "", func() {
		sel := 0
		if a.cfg.Region == "global" {
			sel = 1
		}
		ui.Segmented(c.Key("region"), &sel, "中国大陆线路", "全球线路").Label("线路").OnChange(func() {
			r := []string{"cn", "global"}[sel]
			if r != a.cfg.Region {
				a.cfg.Region = r
				a.changed(a.cfg)
				a.connect()
			}
		})
		ui.Text(c, "大陆线路用 *.longbridge.cn，在国内更快；连不上时换全球线路。").FontSize(11).TextColor(k.muted)
		night := a.cfg.Overnight
		option(c, k, "美股夜盘行情", "北京时间白天显示美股夜盘价格（长桥 US LV1 已包含）。", &night, func() {
			a.cfg.Overnight = night
			a.changed(a.cfg)
			a.connect()
		})
	})
	section(c, k, "其他", "", func() {
		login := a.loginItem()
		option(c, k, "开机自动启动", "登录 "+plat.name+" 后在"+plat.bar+"常驻。", &login, func() {
			if err := a.setLoginItem(login); err != nil {
				a.settingsMsg, a.settingsErr = "设置开机启动失败："+err.Error(), true
			}
		})
		translucent := a.cfg.Translucent
		option(c, k, "半透明背景", "默认关闭：不透明背景在白色网页上也清楚。", &translucent, func() {
			a.cfg.Translucent = translucent
			a.changed(a.cfg)
			a.setTranslucent(translucent)
		})
		ui.Row(c.Key("pos")).Gap(10).Padding(10, 14).Radius(10).BorderWidth(1).BorderColor(k.line).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "面板位置").FontSize(13).FontWeight(550)
				desc := "拖动面板顶部或底部的空白处即可移动，每块屏幕分别记住；双击那里也能复位。"
				if n := len(a.cfg.PanelPos); n > 0 {
					desc = fmt.Sprintf("已记住 %d 块屏幕上的位置。", n) + desc
				}
				ui.Text(c, desc).FontSize(11).TextColor(k.muted).LineHeight(1.45)
			})
			ui.Button(c.Key("resetpos"), "恢复默认位置").Disabled(len(a.cfg.PanelPos) == 0).OnClick(a.resetPosition)
		})
	})
	a.message(c, k)
}

func orDash(s string) string {
	if s == "" {
		return "（仅图标）"
	}
	return s
}

// tickerList are the 常用股票, to move and remove, and a search to add
// one: a code, or a name the app knows.
func (a *app) tickerList(c *ui.Context, k ink) {
	ui.Column(c).Gap(6).Children(func() {
		if len(a.cfg.Tickers) == 0 {
			ui.Text(c, "还没有常用股票：菜单里会提示添加"+map[bool]string{true: "，" + plat.bar + "显示自选的第一只。", false: "。"}[plat.title]).FontSize(12).TextColor(k.muted)
		}
		n := len(a.cfg.Tickers)
		for i, s := range a.cfg.Tickers {
			ui.Row(c.Key(s)).Gap(6).Padding(5, 10).Radius(8).Background(k.card).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprint(i+1)).FontSize(11).TextColor(k.faint).Width(14)
				ui.Text(c, a.menuLabel(s)).FontSize(12).Grow(1).Shrink(1).SingleLine()
				ui.Button(c.Key("up"), "上移").Disabled(i == 0).OnClick(func() { a.movePin(i, -1) })
				ui.Button(c.Key("down"), "下移").Disabled(i == n-1).OnClick(func() { a.movePin(i, 1) })
				ui.Button(c.Key("rm"), "移除").OnClick(func() { a.unpin(s) })
			})
		}
		full := n >= maxPins
		ui.Row(c.Key("add")).Gap(8).AlignItems(ui.Center).Children(func() {
			ph := "搜索代码或名称，例如 700、AAPL、茅台"
			if full {
				ph = fmt.Sprintf("已满 %d 只", maxPins)
			}
			ui.TextInput(c.Key("ticker"), &a.editTicker).Label("添加常用股票").Placeholder(ph).Grow(1).Disabled(full).OnSubmit(a.addTicker)
			ui.Button(c.Key("addticker"), "添加").Disabled(full || strings.TrimSpace(a.editTicker) == "").OnClick(a.addTicker)
		})
		if q := strings.TrimSpace(a.editTicker); q != "" && !full {
			for _, h := range a.tickerHits(q) {
				ui.Row(c.Key("hit"+h.Symbol)).Gap(8).Padding(4, 10).Radius(7).AlignItems(ui.Center).Cursor(ui.CursorPointer).
					Label("添加 " + h.Name).OnClick(func() { a.addPin(h.Symbol) }).Children(func() {
					ui.Text(c, "＋").FontSize(12).TextColor(k.link)
					ui.Text(c, h.Name).FontSize(12).Grow(1).SingleLine()
					ui.Text(c, h.Symbol).FontSize(11).TextColor(k.faint)
				})
			}
		}
	})
}

// tickerHits are what the field's text finds: a code first, then names.
func (a *app) tickerHits(q string) []feed.Watched {
	var out []feed.Watched
	seen := map[string]bool{}
	for _, sym := range codeCandidates(q) {
		if !seen[sym] && !a.pinned(sym) {
			seen[sym] = true
			out = append(out, feed.Watched{Symbol: sym, Name: a.name(sym)})
		}
	}
	for _, h := range searchKnown(q, a.known(), 5) {
		if !seen[h.Symbol] && !a.pinned(h.Symbol) {
			seen[h.Symbol] = true
			out = append(out, h)
		}
	}
	return out[:min(len(out), 5)]
}

// addTicker adds what the field finds first.
func (a *app) addTicker() {
	q := strings.TrimSpace(a.editTicker)
	hits := a.tickerHits(q)
	if len(hits) == 0 {
		a.settingsMsg, a.settingsErr = "找不到「"+q+"」，填代码试试：700、AAPL、600519", true
		return
	}
	a.addPin(hits[0].Symbol)
}

func (a *app) addPin(sym string) {
	if err := a.pin(sym); err != "" {
		a.settingsMsg, a.settingsErr = err, true
		return
	}
	a.editTicker = ""
	a.settingsMsg, a.settingsErr = "已添加 "+a.name(sym)+"（"+sym+"）到常用股票", false
}

func (a *app) shortcutField(c *ui.Context, k ink) {
	ui.Row(c.Key("short")).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c.Key("shortcut"), &a.editShortcut).Label("快捷键").Placeholder(defaultShortcut).Grow(1).
			OnSubmit(a.applyShortcut)
		ui.Button(c.Key("applyshort"), "应用").Disabled(strings.TrimSpace(a.editShortcut) == a.cfg.Shortcut).OnClick(a.applyShortcut)
	})
	ui.Text(c, "当前："+keys(a.cfg.Shortcut)).FontSize(11).TextColor(k.muted)
}

func (a *app) applyShortcut() {
	s := strings.TrimSpace(a.editShortcut)
	if s == "" || s == a.cfg.Shortcut {
		return
	}
	if err := a.setShort(s); err != nil {
		a.settingsMsg, a.settingsErr = "快捷键不可用："+err.Error(), true
		return
	}
	a.cfg.Shortcut = s
	a.changed(a.cfg)
	a.settingsMsg, a.settingsErr = "快捷键已改为 "+keys(s), false
}

// The account

func (a *app) tabAccount(c *ui.Context, k ink) {
	section(c, k, "长桥 OpenAPI 凭证", "到 open.longbridge.com 登录，在「用户中心 › 应用凭证」里复制 App Key、App Secret 和 Access Token。保存前会连一次长桥验证。LB Bar 只读行情和持仓，不会下单。", func() {
		if a.credErr != "" && a.credFrom != "keychain" {
			ui.Column(c.Key("kcerr")).Gap(8).Padding(10, 12).Radius(10).Background(k.danger.Alpha(0.08)).Children(func() {
				ui.Text(c, a.credErr+"。"+storeAdvice()).
					FontSize(12).TextColor(k.danger).LineHeight(1.45)
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c.Key("retry"), "重试读取").OnClick(a.reloadCreds)
					if a.cfg.CredStore != "file" {
						ui.Button(c.Key("tofile"), "改用本地文件").OnClick(func() { a.setCredStore("file") })
					}
				})
			})
		}
		if a.creds.Complete() && !a.changingCred {
			a.savedCreds(c.Key("saved"), k)
		} else {
			a.credForm(c.Key("form"), k)
		}
		a.message(c, k)
	})
	if a.creds.Complete() && !a.changingCred {
		a.expirySection(c, k)
	}
	section(c, k, "保存位置", "", func() {
		sel := 0
		if a.cfg.CredStore == "file" {
			sel = 1
		}
		ui.Segmented(c.Key("store"), &sel, plat.store+"（推荐）", "本地文件").Label("保存位置").OnChange(func() {
			mode := ""
			if sel == 1 {
				mode = "file"
			}
			a.setCredStore(mode)
		})
		desc := credStoreDesc()
		if a.cfg.CredStore == "file" {
			desc = "备用方式：保存在 " + a.keys.path() + "（权限 0600，只有你的账户能读），没有系统加密。"
		}
		ui.Text(c, desc).FontSize(11).TextColor(k.muted).LineHeight(1.45)
	})
}

func (a *app) savedCreds(c *ui.Context, k ink) {
	ui.Column(c).Gap(8).Padding(12, 14).Radius(10).BorderWidth(1).BorderColor(k.line).Children(func() {
		for _, r := range [][2]string{{"App Key", a.creds.AppKey}, {"App Secret", a.creds.AppSecret}, {"Access Token", a.creds.AccessToken}} {
			ui.Row(c.Key(r[0])).Gap(10).Children(func() {
				ui.Text(c, r[0]).FontSize(12).TextColor(k.muted).Width(96)
				ui.Text(c, "已保存  "+mask(r[1])).FontSize(13).FontWeight(550).Grow(1)
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "存于："+storeName(a.credFrom)).FontSize(11).TextColor(k.muted).Grow(1)
			ui.Button(c.Key("change"), "更换").OnClick(func() {
				a.changingCred, a.editKey, a.editSecret, a.editToken = true, "", "", ""
				a.settingsMsg = ""
			})
			if a.credFrom != "env" {
				ui.Button(c.Key("delete"), "删除").OnClick(a.deleteCreds)
			}
		})
	})
}

func (a *app) credForm(c *ui.Context, k ink) {
	ui.Column(c).Gap(8).Children(func() {
		fields := []struct {
			label, ph string
			v         *string
		}{{"App Key", "App Key", &a.editKey}, {"App Secret", "App Secret", &a.editSecret}, {"Access Token", "Access Token（很长，一般以 eyJ 开头）", &a.editToken}}
		for i, f := range fields {
			ui.Row(c.Key(f.label)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, f.label).FontSize(12).TextColor(k.muted).Width(96)
				in := ui.TextInput(c.Key("in"), f.v).Label(f.label).Placeholder(f.ph).Password().Grow(1).Disabled(a.checking).OnSubmit(a.saveCreds)
				if i == 0 && (!a.configured() || a.changingCred) {
					in.AutoFocus()
				}
			})
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Spacer(c)
			if a.changingCred {
				ui.Button(c.Key("cancel"), "取消").OnClick(func() { a.changingCred = false; a.settingsMsg = "" })
			}
			ok := strings.TrimSpace(a.editKey) != "" && strings.TrimSpace(a.editSecret) != "" && strings.TrimSpace(a.editToken) != ""
			ui.PrimaryButton(c.Key("save"), "验证并保存").Disabled(a.checking || !ok).OnClick(a.saveCreds)
		})
		ui.Text(c, "Access Token 有有效期，过期后在开发者中心重新生成，再到这里更换。").FontSize(11).TextColor(k.faint)
	})
}

// expirySection shows when the token expires, read from it, or as entered.
func (a *app) expirySection(c *ui.Context, k ink) {
	section(c, k, "Access Token 有效期", "", func() {
		t, ok := a.expiry()
		n, _ := a.daysLeft()
		switch {
		case ok && n < 0:
			ui.Text(c, "已于 "+t.In(feed.Shanghai).Format("2006-01-02 15:04")+"（北京时间）过期").FontSize(13).TextColor(k.danger)
		case ok:
			col := k.text
			if n < remindDays {
				col = k.gold
			}
			ui.Text(c, fmt.Sprintf("%s 到期（北京时间），还有 %d 天", t.In(feed.Shanghai).Format("2006-01-02 15:04"), n)).FontSize(13).TextColor(col)
		default:
			ui.Text(c, "读不出到期时间。填上开发者中心显示的到期日，到期前 14 天会提醒。").FontSize(12).TextColor(k.muted)
		}
		if _, fromToken := tokenExpiry(a.creds.AccessToken); fromToken {
			ui.Text(c, "到期时间读自 Token 本身。到期前 14 天面板顶部会提示，前 7 天每天发一次通知。").FontSize(11).TextColor(k.muted)
		} else {
			ui.Row(c.Key("exp")).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c.Key("expiry"), &a.editExpiry).Label("到期日").Placeholder("2027-01-19").Width(140).OnSubmit(a.saveExpiry)
				ui.Button(c.Key("saveexp"), "保存").OnClick(a.saveExpiry)
			})
		}
		ui.Link(c.Key("center"), "打开长桥开发者中心 ›", "https://open.longbridge.com/").FontSize(12)
	})
}

func (a *app) saveExpiry() {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(a.editExpiry), feed.Shanghai)
	if err != nil {
		a.settingsMsg, a.settingsErr = "日期格式：2027-01-19", true
		return
	}
	a.cfg.TokenExpires = t.Add(24*time.Hour - time.Second).Format(time.RFC3339)
	a.changed(a.cfg)
	a.settingsMsg, a.settingsErr = "已保存到期日", false
}

// saveCreds connects with the credentials typed; that working, saves
// them and uses the connection.
func (a *app) saveCreds() {
	cr := feed.Creds{AppKey: strings.TrimSpace(a.editKey), AppSecret: strings.TrimSpace(a.editSecret), AccessToken: strings.TrimSpace(a.editToken)}
	if !cr.Complete() || a.checking || a.dial == nil {
		return
	}
	a.checking, a.settingsMsg, a.settingsErr = true, "正在连接长桥验证…", false
	dial, opts := a.dial, a.options()
	a.spawn(func() {
		p, err := dial(cr, opts)
		if err == nil {
			_, err = p.Markets(context.Background())
		}
		a.post(func() {
			a.checking = false
			if err != nil {
				if p != nil {
					_ = p.Close()
				}
				a.settingsMsg, a.settingsErr = "验证失败："+feed.Friendly(err).Error(), true
				return
			}
			from, warn, serr := a.keys.save(cr, a.cfg.CredStore)
			if serr != nil {
				_ = p.Close()
				a.settingsMsg, a.settingsErr = "保存失败："+serr.Error(), true
				return
			}
			a.creds, a.credFrom, a.credErr = cr, from, ""
			a.changingCred, a.editKey, a.editSecret, a.editToken = false, "", "", ""
			if from == "file" && a.cfg.CredStore != "file" {
				a.cfg.CredStore = "file"
			}
			a.cfg.TokenExpires, a.cfg.Reminded = "", ""
			a.changed(a.cfg)
			msg := "已验证并保存到" + storeName(from)
			if warn != "" {
				msg = "已验证；" + warn + "，改存在本地文件"
			}
			a.settingsMsg, a.settingsErr = msg, warn != ""
			a.disconnect()
			a.seq++
			a.use(p)
		})
	})
}

func (a *app) deleteCreds() {
	if err := a.keys.remove(); err != nil {
		a.settingsMsg, a.settingsErr = "删除失败："+err.Error(), true
		return
	}
	a.disconnect()
	a.seq++
	a.creds, a.credFrom = feed.Creds{}, ""
	a.groups, a.positions, a.balances = nil, nil, nil
	a.cfg.TokenExpires = ""
	a.changed(a.cfg)
	a.settingsMsg, a.settingsErr = "已删除凭证", false
	a.trayChanged()
}

func (a *app) reloadCreds() {
	st := a.keys.load(a.cfg.CredStore)
	a.credErr = st.err
	if st.creds.Complete() {
		a.creds, a.credFrom = st.creds, st.from
		a.settingsMsg, a.settingsErr = "已读取凭证，正在连接…", false
		a.connect()
	} else if st.err != "" {
		a.settingsMsg, a.settingsErr = st.err, true
	}
}

// setCredStore moves the credentials to the keychain, or the file.
func (a *app) setCredStore(mode string) {
	if mode == a.cfg.CredStore {
		return
	}
	a.cfg.CredStore = mode
	a.changed(a.cfg)
	if !a.creds.Complete() || a.credFrom == "env" {
		return
	}
	from, warn, err := a.keys.save(a.creds, mode)
	switch {
	case err != nil:
		a.settingsMsg, a.settingsErr = "移动失败："+err.Error(), true
	case warn != "":
		a.credFrom, a.settingsMsg, a.settingsErr = from, warn+"，仍存在本地文件", true
	default:
		a.credFrom, a.settingsMsg, a.settingsErr = from, "已移到"+storeName(from), false
		a.credErr = ""
	}
}

// Alerts

func (a *app) tabAlerts(c *ui.Context, k ink) {
	section(c, k, "新建提醒", "价格提醒在本机检查：LB Bar 在"+plat.bar+"运行时，行情满足条件就发"+plat.notify+"。在列表里选中股票后按 "+cmdKey()+"N 会带上它。", func() {
		ui.Row(c.Key("form")).Gap(8).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("sym"), &a.alertSym).Label("股票").Placeholder("700 / AAPL / 茅台").Width(130).OnSubmit(a.addAlert)
			if a.focusAlert && a.alertSym == "" {
				in.AutoFocus()
			}
			labels := make([]string, len(alerts.Kinds))
			for i, kd := range alerts.Kinds {
				labels[i] = kd.Label()
			}
			ui.Segmented(c.Key("kind"), &a.alertKind, labels...).Label("条件")
		})
		ui.Row(c.Key("form2")).Gap(8).AlignItems(ui.Center).Children(func() {
			ph := "价格，如 500"
			if a.alertKind >= 2 {
				ph = "百分比，如 3"
			}
			v := ui.TextInput(c.Key("value"), &a.alertValue).Label("数值").Placeholder(ph).Width(130).OnSubmit(a.addAlert)
			if a.focusAlert && a.alertSym != "" {
				v.AutoFocus()
			}
			if a.alertKind >= 2 {
				ui.Text(c, "%").FontSize(12).TextColor(k.muted)
			}
			ui.Checkbox(c.Key("repeat"), &a.alertRepeat, "重复提醒（回落后再次满足时）")
			ui.Spacer(c)
			ui.PrimaryButton(c.Key("add"), "添加").Disabled(strings.TrimSpace(a.alertSym) == "" || strings.TrimSpace(a.alertValue) == "").OnClick(a.addAlert)
		})
		a.focusAlert = false
		if a.alertMsg != "" {
			col := k.green
			if a.alertErr {
				col = k.danger
			}
			ui.Text(c, a.alertMsg).FontSize(12).TextColor(col)
		}
	})
	section(c, k, "我的提醒", "", func() {
		rules := a.alerts.Rules()
		if len(rules) == 0 {
			ui.Text(c, "还没有提醒。").FontSize(12).TextColor(k.muted)
		}
		for _, r := range rules {
			ui.Row(c.Key(r.ID)).Gap(10).Padding(8, 12).Radius(9).BorderWidth(1).BorderColor(k.line).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBell).FontSize(13).TextColor(k.faint)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					name := r.Name
					if name == "" {
						name = a.name(r.Symbol)
					}
					ui.Text(c, name+"  "+r.Symbol).FontSize(13).SingleLine()
					sub := r.Describe()
					if r.Repeat {
						sub += " · 重复"
					}
					if q := a.quotes[r.Symbol]; q != nil && q.Last > 0 {
						_, rate := q.Change()
						sub += " · 现价 " + price(q.Last) + " " + pct(rate)
					}
					if !r.Fired.IsZero() {
						sub += " · 上次 " + clock(r.Fired, a.now())
					}
					ui.Text(c, sub).FontSize(11).TextColor(k.muted).SingleLine()
				})
				on := r.Enabled
				if ui.Switch(c.Key("on"), &on).Label("启用 " + r.Symbol).Changed() {
					_ = a.alerts.SetEnabled(r.ID, on)
					a.refreshQuotes(true)
				}
				ui.Button(c.Key("rm"), "删除").OnClick(func() { _ = a.alerts.Remove(r.ID) })
			})
		}
	})
	section(c, k, "通知", "", func() {
		on := a.cfg.Notify
		option(c, k, "用"+plat.notify+"提醒", notifyDesc(), &on, func() {
			a.cfg.Notify = on
			a.changed(a.cfg)
		})
	})
}

func (a *app) tabKeys(c *ui.Context, k ink) {
	section(c, k, "唤起快捷键", "", func() { a.shortcutField(c, k) })
	section(c, k, "面板里的快捷键", "全键盘操作；任何时候按 "+cmdKey()+"/ 也能看这张表。", func() {
		ui.Column(c).Gap(1).Children(func() {
			for i, g := range shortcutGroups() {
				if g[0] != "" {
					ui.Text(c, g[0]).FontSize(11).FontWeight(600).TextColor(k.muted).Margin(8, 0, 2, 0)
				}
				ui.Row(c.Key(i)).Gap(14).Padding(3, 0).AlignItems(ui.Center).Children(func() {
					ui.Text(c, g[1]).FontSize(12).FontWeight(600).TextColor(k.link).Width(150).SingleLine()
					ui.Text(c, g[2]).FontSize(12).Grow(1).SingleLine()
				})
			}
		})
	})
	a.message(c, k)
}

func (a *app) tabAbout(c *ui.Context, k ink) {
	section(c, k, "LB Bar "+version, plat.bar+"里的长桥行情："+keys(a.cfg.Shortcut)+" 唤起，自选实时推送、分时和 K 线、只读持仓、价格提醒。非长桥官方产品。", func() {
		ui.Row(c).Gap(14).Children(func() {
			ui.Link(c.Key("open"), "长桥开发者中心 ›", "https://open.longbridge.com/").FontSize(12)
			ui.Link(c.Key("docs"), "OpenAPI 文档 ›", "https://open.longbridge.com/docs").FontSize(12)
		})
	})
	section(c, k, "安全", "LB Bar 只读：只调用行情、自选、新闻、资产和持仓查询接口，从不调用下单、改单、撤单接口。凭证只存在本机"+plat.store+"（或你选的本地文件）里，只发给长桥。", func() {})
	section(c, k, "数据保存在哪", "", func() {
		rows := [][2]string{
			{"设置", a.dataDir + "/config.json"},
			{"价格提醒", a.dataDir + "/alerts.json"},
			{"凭证", storeName(a.credFrom)},
		}
		for _, r := range rows {
			ui.Row(c.Key(r[0])).Gap(12).Children(func() {
				ui.Text(c, r[0]).FontSize(12).TextColor(k.muted).Width(80)
				ui.Text(c, r[1]).FontSize(12).Grow(1).Selectable()
			})
		}
	})
	ui.Row(c.Key("quitrow")).Children(func() {
		ui.Button(c.Key("quit"), "退出 LB Bar  "+cmdKey()+"Q").OnClick(a.quit)
	})
}
