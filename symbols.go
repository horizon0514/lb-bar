package main

import (
	"sort"
	"strings"
	"unicode"

	"lb-bar/feed"
)

// Search finds securities by code (700, 00700.HK, AAPL, 600519), name
// (腾讯, 茅台, Apple) or a few aliases (tencent, tx). The OpenAPI has no
// free-text search, so it looks in what the app knows: the watchlist, the
// holdings, the US securities the API lists, and aliases of well-known
// names; a code that looks like one is checked with the API's static info.

// alias are well-known names and nicknames, for what the API can't search.
var aliases = []struct {
	keys   []string
	symbol string
	name   string
}{
	{[]string{"tencent", "腾讯", "tx", "txkg", "企鹅"}, "700.HK", "腾讯控股"},
	{[]string{"alibaba", "阿里", "阿里巴巴", "albb", "ali"}, "9988.HK", "阿里巴巴-W"},
	{[]string{"alibaba", "阿里", "阿里巴巴", "baba"}, "BABA.US", "阿里巴巴"},
	{[]string{"meituan", "美团", "mt"}, "3690.HK", "美团-W"},
	{[]string{"xiaomi", "小米", "xm", "mi"}, "1810.HK", "小米集团-W"},
	{[]string{"moutai", "茅台", "贵州茅台", "mt", "gzmt"}, "600519.SH", "贵州茅台"},
	{[]string{"catl", "宁德", "宁德时代", "ndsd"}, "300750.SZ", "宁德时代"},
	{[]string{"byd", "比亚迪"}, "1211.HK", "比亚迪股份"},
	{[]string{"byd", "比亚迪"}, "002594.SZ", "比亚迪"},
	{[]string{"jd", "京东"}, "9618.HK", "京东集团-SW"},
	{[]string{"jd", "京东"}, "JD.US", "京东"},
	{[]string{"pdd", "拼多多", "temu"}, "PDD.US", "拼多多"},
	{[]string{"baidu", "百度", "bd"}, "9888.HK", "百度集团-SW"},
	{[]string{"baidu", "百度", "bidu"}, "BIDU.US", "百度"},
	{[]string{"netease", "网易", "wy"}, "9999.HK", "网易-S"},
	{[]string{"kuaishou", "快手", "ks"}, "1024.HK", "快手-W"},
	{[]string{"hkex", "港交所", "香港交易所"}, "388.HK", "香港交易所"},
	{[]string{"hsbc", "汇丰"}, "5.HK", "汇丰控股"},
	{[]string{"aia", "友邦"}, "1299.HK", "友邦保险"},
	{[]string{"china mobile", "中国移动", "中移动"}, "941.HK", "中国移动"},
	{[]string{"icbc", "工商银行", "工行"}, "1398.HK", "工商银行"},
	{[]string{"cmb", "招商银行", "招行"}, "600036.SH", "招商银行"},
	{[]string{"smic", "中芯", "中芯国际"}, "981.HK", "中芯国际"},
	{[]string{"popmart", "泡泡玛特"}, "9992.HK", "泡泡玛特"},
	{[]string{"hsi", "恒指", "恒生指数"}, "HSI.HK", "恒生指数"},
	{[]string{"hstech", "恒生科技", "恒科"}, "HSTECH.HK", "恒生科技指数"},
	{[]string{"nasdaq", "纳指", "纳斯达克"}, ".IXIC.US", "纳斯达克综合指数"},
	{[]string{"dow", "道指", "道琼斯"}, ".DJI.US", "道琼斯指数"},
	{[]string{"s&p", "spx", "标普", "标普500"}, ".SPX.US", "标普500指数"},
	{[]string{"上证", "上证指数", "sh000001"}, "000001.SH", "上证指数"},
	{[]string{"深证成指", "深成指"}, "399001.SZ", "深证成指"},
	{[]string{"创业板", "创业板指"}, "399006.SZ", "创业板指"},
	{[]string{"apple", "苹果", "aapl"}, "AAPL.US", "苹果"},
	{[]string{"nvidia", "英伟达", "nvda"}, "NVDA.US", "英伟达"},
	{[]string{"tesla", "特斯拉", "tsla"}, "TSLA.US", "特斯拉"},
	{[]string{"microsoft", "微软", "msft"}, "MSFT.US", "微软"},
	{[]string{"google", "谷歌", "alphabet"}, "GOOGL.US", "谷歌-A"},
	{[]string{"amazon", "亚马逊"}, "AMZN.US", "亚马逊"},
	{[]string{"meta", "facebook", "脸书"}, "META.US", "Meta Platforms"},
}

var suffixes = map[string]bool{"HK": true, "US": true, "SH": true, "SZ": true, "SG": true}

// codeCandidates are the symbols a query that looks like a code may mean.
func codeCandidates(q string) []string {
	s := strings.ToUpper(strings.TrimSpace(q))
	if s == "" {
		return nil
	}
	if i := strings.LastIndexByte(s, '.'); i > 0 && suffixes[s[i+1:]] {
		code, mk := s[:i], s[i+1:]
		if mk == "HK" && allDigits(code) {
			code = strings.TrimLeft(code, "0")
		}
		if code == "" {
			return nil
		}
		return []string{code + "." + mk}
	}
	switch {
	case allDigits(s) && len(s) <= 5:
		code := strings.TrimLeft(s, "0")
		if code == "" {
			return nil
		}
		return []string{code + ".HK"}
	case allDigits(s) && len(s) == 6:
		switch s[0] {
		case '6', '9', '5':
			return []string{s + ".SH"}
		case '0', '2', '3', '1':
			return []string{s + ".SZ"}
		}
		return nil
	case isTicker(s):
		return []string{s + ".US"}
	}
	return nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// isTicker: a US ticker, like AAPL, BRK.B is BRK-B? Letters, at most 5,
// with a dot or a dash inside, or an index (.DJI).
func isTicker(s string) bool {
	if len(s) == 0 || len(s) > 7 {
		return false
	}
	letters := 0
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			letters++
		case (r == '.' || r == '-') && i > 0 && i < len(s)-1:
		case r == '.' && i == 0:
		default:
			return false
		}
	}
	return letters > 0 && letters <= 5
}

// hit is a search result, with its score: higher first.
type hit struct {
	feed.Watched
	score int
}

// normalize folds a name for matching: lower case, no spaces or dashes.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || r == '-' || r == '_' || r == '·' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// searchKnown finds q in the securities the app knows (by symbol, with
// names; a directory entry's name may carry its English one after a NUL),
// and in the aliases. At most limit, best first.
func searchKnown(q string, known []feed.Watched, limit int) []feed.Watched {
	nq := normalize(q)
	if nq == "" {
		return nil
	}
	uq := strings.ToUpper(strings.TrimSpace(q))
	best := map[string]hit{}
	add := func(w feed.Watched, score int) {
		if score <= 0 {
			return
		}
		if h, ok := best[w.Symbol]; !ok || score > h.score {
			if ok && w.Name == "" {
				w.Name = h.Name
			}
			best[w.Symbol] = hit{w, score}
		}
	}
	nameScore := func(name string) int {
		n := normalize(name)
		switch {
		case n == "":
			return 0
		case n == nq:
			return 70
		case strings.HasPrefix(n, nq):
			return 55
		case len([]rune(nq)) >= 2 && strings.Contains(n, nq):
			return 40
		}
		return 0
	}
	for _, w := range known {
		code := feed.Code(w.Symbol)
		names := strings.Split(w.Name, "\x00")
		disp := feed.Watched{Symbol: w.Symbol, Name: names[0]}
		switch {
		case strings.EqualFold(w.Symbol, uq):
			add(disp, 100)
		case strings.EqualFold(code, uq):
			add(disp, 90)
		case len(uq) >= 2 && strings.HasPrefix(strings.ToUpper(code), uq):
			add(disp, 60-len(code))
		}
		for _, n := range names {
			add(disp, nameScore(n))
		}
	}
	for _, al := range aliases {
		for _, k := range al.keys {
			nk := normalize(k)
			switch {
			case nk == nq:
				add(feed.Watched{Symbol: al.symbol, Name: al.name}, 80)
			case len([]rune(nq)) >= 2 && strings.HasPrefix(nk, nq):
				add(feed.Watched{Symbol: al.symbol, Name: al.name}, 45)
			}
		}
	}
	hits := make([]hit, 0, len(best))
	for _, h := range best {
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].Symbol < hits[j].Symbol
	})
	out := make([]feed.Watched, 0, min(limit, len(hits)))
	for _, h := range hits[:min(limit, len(hits))] {
		out = append(out, h.Watched)
	}
	return out
}

// quoteURL is a security's quote page on the Longbridge website, which
// the Longbridge app, where installed, can take over. There is no
// documented deep link into the desktop app.
func quoteURL(symbol string) string {
	return "https://longbridge.com/zh-CN/quote/" + symbol
}
