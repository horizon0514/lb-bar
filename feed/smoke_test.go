package feed

import (
	"context"
	"os"
	"testing"
	"time"
)

// envCreds are the credentials of the environment, as the SDK names them.
func envCreds() Creds {
	get := func(names ...string) string {
		for _, n := range names {
			if v := os.Getenv(n); v != "" {
				return v
			}
		}
		return ""
	}
	return Creds{
		AppKey:      get("LONGBRIDGE_APP_KEY", "LONGPORT_APP_KEY"),
		AppSecret:   get("LONGBRIDGE_APP_SECRET", "LONGPORT_APP_SECRET"),
		AccessToken: get("LONGBRIDGE_ACCESS_TOKEN", "LONGPORT_ACCESS_TOKEN"),
	}
}

// TestRealAPI runs against Longbridge with the credentials of the
// environment, read only: no order is placed, changed or cancelled. It
// logs counts and shapes, never the account's amounts. Skipped without
// credentials, and with -short.
func TestRealAPI(t *testing.T) {
	creds := envCreds()
	if !creds.Complete() || testing.Short() {
		t.Skip("set LONGPORT_APP_KEY, LONGPORT_APP_SECRET and LONGPORT_ACCESS_TOKEN to run")
	}
	region := os.Getenv("LBBAR_REGION")
	if region == "" {
		region = "cn"
	}
	if region == "global" {
		region = ""
	}
	p, err := Dial(creds, Options{Region: region, Overnight: true})
	if err != nil {
		t.Fatalf("dial: %v", Friendly(err))
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	gs, err := p.Groups(ctx)
	if err != nil {
		t.Fatalf("groups: %v", err)
	}
	n := 0
	for _, g := range gs {
		n += len(g.Securities)
	}
	t.Logf("watchlist: %d groups, %d securities", len(gs), n)

	syms := []string{"700.HK", "AAPL.US", "600519.SH"}
	secs, err := p.Static(ctx, syms)
	if err != nil || len(secs) != 3 || secs[0].Name == "" || secs[0].LotSize == 0 {
		t.Fatalf("static: %v %+v", err, secs)
	}
	qs, err := p.Quotes(ctx, syms)
	if err != nil || len(qs) != 3 {
		t.Fatalf("quotes: %v %d", err, len(qs))
	}
	for _, q := range qs {
		if q.Last <= 0 || q.PrevClose <= 0 {
			t.Errorf("quote %s: last %v prev %v", q.Symbol, q.Last, q.PrevClose)
		}
	}
	st, err := p.Stats(ctx, syms)
	if err != nil || len(st) != 3 || st[0].MarketCap <= 0 {
		t.Errorf("stats: %v %+v", err, st)
	}
	pts, err := p.Intraday(ctx, "700.HK")
	if err != nil {
		t.Errorf("intraday: %v", err)
	}
	t.Logf("intraday 700.HK: %d points", len(pts))
	for _, per := range []Period{Day, Week, Month} {
		cs, err := p.Candles(ctx, "AAPL.US", per, 30)
		if err != nil || len(cs) == 0 || cs[len(cs)-1].Close <= 0 {
			t.Errorf("candles %v: %v %d", per, err, len(cs))
		}
	}
	ns, err := p.News(ctx, "700.HK")
	if err != nil {
		t.Errorf("news: %v", err)
	}
	t.Logf("news 700.HK: %d items", len(ns))
	ms, err := p.Markets(ctx)
	if err != nil || len(ms) == 0 {
		t.Errorf("markets: %v", err)
	}
	for _, m := range ms {
		t.Logf("market %s: %s", m.Market, m.Phase.Label(m.Market))
	}
	dir, err := p.Directory(ctx)
	if err != nil || len(dir) < 1000 {
		t.Errorf("directory: %v %d", err, len(dir))
	}
	t.Logf("US directory: %d securities", len(dir))

	// Subscribe, and wait a while for a push: none comes when every
	// market is closed, which is not a failure.
	got := make(chan Push, 16)
	p.OnPush(func(ps Push) {
		select {
		case got <- ps:
		default:
		}
	})
	if err := p.Subscribe(ctx, syms); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	select {
	case ps := <-got:
		t.Logf("push: %s session %d", ps.Symbol, ps.Session)
	case <-time.After(20 * time.Second):
		t.Log("no push in 20 s (markets closed?)")
	}
	if err := p.Unsubscribe(ctx, syms); err != nil {
		t.Errorf("unsubscribe: %v", err)
	}

	// The account, read only. Only how many: no amounts in the log.
	pos, err := p.Positions(ctx)
	if err != nil {
		t.Errorf("positions: %v", err)
	}
	bal, err := p.Balances(ctx)
	if err != nil {
		t.Errorf("balances: %v", err)
	}
	t.Logf("account: %d positions, %d balances (amounts not logged)", len(pos), len(bal))
	var curs []string
	for _, b := range bal {
		curs = append(curs, b.Currency)
	}
	for _, x := range pos {
		curs = append(curs, x.Currency)
	}
	t.Logf("account currencies: %v", curs)
	rs, err := p.Rates(ctx)
	if err != nil || len(rs) == 0 {
		t.Errorf("rates: %d, %v", len(rs), err)
	}
	for _, r := range rs { // market data, not the account's
		if r.From == "HKD" || r.From == "CNY" || r.From == "CNH" {
			t.Logf("rate: 1 %s = %.5f %s (1 %s = %.4f %s)", r.From, r.Rate, r.To, r.To, 1/r.Rate, r.From)
		}
	}
	t.Logf("%d rates", len(rs))
	cs, err := p.Candles(ctx, "700.HK", Day, 300)
	t.Logf("700.HK day candles asked 300: %d, %v", len(cs), err)
}
