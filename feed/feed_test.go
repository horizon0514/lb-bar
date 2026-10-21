package feed

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestScheduledPhase(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, Shanghai)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct {
		market, at string
		want       Phase
	}{
		{"HK", "2026-10-20 09:20", PreOpen},
		{"HK", "2026-10-20 10:00", Trading},
		{"HK", "2026-10-20 12:30", Break},
		{"HK", "2026-10-20 16:05", PostClose},
		{"HK", "2026-10-20 17:00", Closed},
		{"HK", "2026-10-24 10:00", Closed}, // Saturday
		{"CN", "2026-10-20 09:20", PreOpen},
		{"CN", "2026-10-20 14:00", Trading},
		{"CN", "2026-10-20 11:45", Break},
		// The US in daylight saving time until 1 November: 21:30–04:00 Beijing.
		{"US", "2026-10-20 21:40", Trading},
		{"US", "2026-10-20 17:00", PreOpen},
		{"US", "2026-10-21 05:00", PostClose},
		{"US", "2026-10-21 14:00", Night},
		// Standard time: 22:30.
		{"US", "2026-11-10 22:00", PreOpen},
		{"US", "2026-11-10 22:40", Trading},
		{"US", "2026-10-25 10:00", Closed}, // Sunday morning
	} {
		if got := ScheduledPhase(c.market, at(c.at)); got != c.want {
			t.Errorf("%s at %s: %v, want %v", c.market, c.at, got.Label(c.market), c.want.Label(c.market))
		}
	}
	if h := Hours("US", at("2026-11-10 12:00")); !strings.Contains(h, "22:30–05:00") {
		t.Errorf("US hours %q", h)
	}
}

func TestQuoteChangeAndApply(t *testing.T) {
	q := Quote{Symbol: "700.HK", Last: 512.5, PrevClose: 506.4}
	abs, rate := q.Change()
	if abs < 6.09 || abs > 6.11 || rate < 0.012 || rate > 0.01205 {
		t.Errorf("%v %v", abs, rate)
	}
	q.Apply(Push{Symbol: "700.HK", Last: 515, High: 520, Volume: 10, Time: time.Now()})
	if q.Last != 515 || q.High != 520 {
		t.Errorf("%+v", q)
	}
	q.Apply(Push{Symbol: "700.HK", Last: 9, Session: PreMarket})
	if q.Last != 515 || q.Pre == nil || q.Pre.Last != 9 || q.Pre.PrevClose != 515 {
		t.Errorf("pre %+v %+v", q, q.Pre)
	}
}

func TestMarketOfAndCode(t *testing.T) {
	for sym, want := range map[string][2]string{"700.HK": {"HK", "700"}, "AAPL.US": {"US", "AAPL"}, "600519.SH": {"CN", "600519"}, "000001.SZ": {"CN", "000001"}, "BRK.B.US": {"US", "BRK.B"}} {
		if MarketOf(sym) != want[0] || Code(sym) != want[1] {
			t.Errorf("%s: %s %s", sym, MarketOf(sym), Code(sym))
		}
	}
}

func TestLimiter(t *testing.T) {
	l := &Limiter{Rate: 50, Burst: 2, Concurrent: 2}
	start := time.Now()
	for i := 0; i < 6; i++ {
		done, err := l.Wait(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		done()
	}
	// 2 at once, then 4 at 50 a second: about 80 ms.
	if d := time.Since(start); d < 60*time.Millisecond {
		t.Errorf("6 calls in %v", d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d1, _ := l.Wait(ctx)
	d2, _ := l.Wait(ctx)
	cancel()
	if _, err := l.Wait(ctx); err == nil {
		t.Error("a third ran with two running")
	}
	d1()
	d2()
}

func TestMockIsExample(t *testing.T) {
	m := &Mock{}
	ns, _ := m.News(context.Background(), "700.HK")
	for _, n := range ns {
		if len(n.Title) < 4 || n.Title[:len("示例新闻")] != "示例新闻" {
			t.Errorf("news not marked: %q", n.Title)
		}
	}
	gs, _ := m.Groups(context.Background())
	if len(gs) == 0 {
		t.Fatal("no groups")
	}
}
