package main

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

// LB Bar runs on macOS, Windows and Linux. What differs, for what the app
// says and does: where the icon lives, the credential store, the keys.

// platformInfo is the system the app runs on, as the app speaks of it.
type platformInfo struct {
	goos      string
	name      string // macOS, Windows, Linux
	bar       string // where the icon is: 菜单栏, 通知区域, 托盘
	store     string // the credential store, short
	storeLong string
	notify    string // its notifications
	// title: the system shows text next to the icon. macOS's menu bar
	// does; Linux's AppIndicator label shows on some panels (Ubuntu's,
	// GNOME's with the AppIndicator extension), not on others; Windows'
	// notification area never.
	title bool
}

func platformFor(goos string) platformInfo {
	switch goos {
	case "darwin":
		return platformInfo{goos: goos, name: "macOS", bar: "菜单栏", store: "macOS 钥匙串", storeLong: "macOS「登录」钥匙串", notify: "macOS 通知", title: true}
	case "windows":
		return platformInfo{goos: goos, name: "Windows", bar: "任务栏通知区域", store: "Windows 凭据管理器", storeLong: "Windows 凭据管理器", notify: "Windows 通知"}
	default:
		return platformInfo{goos: goos, name: "Linux", bar: "系统托盘", store: "系统密钥环", storeLong: "系统密钥环（Secret Service，如 GNOME Keyring / KWallet）", notify: "桌面通知", title: true}
	}
}

// plat is the system the app runs on; tests set another with setPlatform.
var plat = platformFor(runtime.GOOS)

// setPlatform makes the app speak as on goos: its names, and its keys.
func setPlatform(goos string) {
	plat = platformFor(goos)
	macHints = goos == "darwin"
	defaultShortcut, fallbackShortcut = shortcutsFor(goos)
}

// shortcutsFor are the global shortcut to summon the panel, and the one
// to try when another app has it. ⌥S on the Mac, where Option-letters
// type symbols no app binds. On Windows and Linux Alt+S is the access key
// of many apps' menus and buttons (Outlook's 发送, Firefox's 历史 menu,
// Explorer's ribbon), which a global shortcut would take from them: Alt+
// Shift+S there, which neither system nor the common apps bind (Alt+Shift
// alone switches the keyboard layout on Windows, with a letter it does
// not), and Alt+Shift+L, as free, when that is taken. Ctrl+Alt is AltGr
// on many European layouts, and Ctrl+Space or Ctrl+Shift switch Chinese
// input methods: neither is used.
func shortcutsFor(goos string) (def, fallback string) {
	if goos == "darwin" {
		return "Alt+S", "Alt+Shift+S"
	}
	return "Alt+Shift+S", "Alt+Shift+L"
}

var (
	defaultShortcut, fallbackShortcut = shortcutsFor(runtime.GOOS)
)

// dataDirName is the app's folder in the user's config folder:
// ~/Library/Application Support/LB Bar, %APPDATA%\LB Bar, or
// ~/.config/lb-bar ($XDG_CONFIG_HOME/lb-bar), as Linux names folders.
func dataDirName(goos string) string {
	if goos == "darwin" || goos == "windows" {
		return "LB Bar"
	}
	return "lb-bar"
}

var modGlyphs = regexp.MustCompile(`[⌃⌥⇧⌘]+`)

// hk spells keys written as the Mac does (⇧⌘C, ⌥←, ⇥, ↵) as the system
// does: Ctrl+Shift+C, Alt+←, Tab, Enter on Windows and Linux.
func hk(s string) string {
	if macHints {
		return s
	}
	s = strings.NewReplacer("⇥", "Tab", "↵", "Enter").Replace(s)
	return modGlyphs.ReplaceAllStringFunc(s, func(m string) string {
		var b strings.Builder
		for _, p := range []struct{ g, name string }{{"⌘", "Ctrl+"}, {"⌃", "Ctrl+"}, {"⌥", "Alt+"}, {"⇧", "Shift+"}} {
			if strings.Contains(m, p.g) && !strings.Contains(b.String(), p.name) {
				b.WriteString(p.name)
			}
		}
		return b.String()
	})
}

// trayTip is the icon's hover text: the markets, and where the system
// shows no text next to the icon, the 常用股票's prices. Windows keeps
// 127 characters of it.
func (a *app) trayTip() string {
	tip := "LB Bar · " + a.trayLine()
	if a.connErr != "" {
		return clip("LB Bar · "+a.connErr, 127)
	}
	if plat.goos != "darwin" { // the label may not show on Linux
		for _, sym := range a.cfg.Tickers {
			tip += "\n" + strings.TrimSpace(a.menuLabel(sym))
		}
	}
	return clip(tip, 127)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// noStore: the system has no credential store (Linux without a Secret
// Service).
func noStore(err error) bool { return errors.Is(err, errors.ErrUnsupported) }

// barDesc is the settings' word on the icon's text.
func barDesc() string {
	switch plat.goos {
	case "darwin":
		return "macOS 菜单栏的文字只能是一种颜色；想看出涨跌，可以打开彩色圆点。"
	case "windows":
		return "Windows 的通知区域只显示图标：价格在图标的悬停提示里，点开图标的菜单是常用股票的实时价格。"
	}
	return "图标旁的文字只在支持 AppIndicator 标签的面板上显示（如 Ubuntu）；其他桌面只显示图标，价格在悬停提示和图标菜单里。"
}

func pinsDesc() string {
	s := fmt.Sprintf("点%s的 LB Bar 图标，菜单里直接列出它们的价格和涨跌，点一只打开它的详情", plat.bar)
	if plat.title {
		s += "；" + plat.bar + "文字也轮播它们"
	} else {
		s += "；图标的悬停提示也显示它们"
	}
	return s + fmt.Sprintf("。最多 %d 只。面板里选中一只按 %sD 也能置顶 / 取消。", maxPins, cmdKey())
}

// storeAdvice is what to do when the credential store fails.
func storeAdvice() string {
	switch plat.goos {
	case "darwin":
		return "通常是 App 更新后 macOS 需要重新授权：点「重试读取」，在弹窗中选「始终允许」；或者改用本地文件保存。"
	case "windows":
		return "点「重试读取」，或者改用本地文件保存。"
	}
	return "请确认已登录桌面会话、密钥环已解锁（GNOME Keyring、KWallet 等），再点「重试读取」；没有密钥环的系统请改用本地文件保存。"
}

func credStoreDesc() string {
	switch plat.goos {
	case "darwin":
		return "三项凭证作为一条记录存在钥匙串里，只授权一次。App 用固定的签名标识，更新后仍能读到；万一要求授权，选「始终允许」。"
	case "windows":
		return "三项凭证作为一条记录存在 Windows 凭据管理器里（普通凭据「dev.horizon.lbbar/longbridge-credentials」），由系统按你的账户加密。"
	}
	return "三项凭证作为一条记录存在系统密钥环里（Secret Service），由密钥环加密；没有密钥环时自动改用本地文件。"
}

func notifyDesc() string {
	if plat.goos == "darwin" {
		return "第一次提醒时 macOS 会问是否允许通知。关掉后只在面板底部提示。"
	}
	return "关掉后只在面板底部提示。"
}
