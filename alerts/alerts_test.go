package alerts

import (
	"path/filepath"
	"testing"
	"time"

	"lb-bar/feed"
)

func TestMet(t *testing.T) {
	q := feed.Quote{Last: 103, PrevClose: 100}
	for _, c := range []struct {
		r    Rule
		want bool
	}{
		{Rule{Kind: Above, Value: 103}, true}, {Rule{Kind: Above, Value: 103.01}, false},
		{Rule{Kind: Below, Value: 103}, true}, {Rule{Kind: Below, Value: 102}, false},
		{Rule{Kind: Rise, Value: 3}, true}, {Rule{Kind: Rise, Value: 3.5}, false},
		{Rule{Kind: Fall, Value: 3}, false},
	} {
		if got := c.r.Met(q); got != c.want {
			t.Errorf("%s: %v", c.r.Describe(), got)
		}
	}
	if (Rule{Kind: Fall, Value: 2}).Met(feed.Quote{Last: 97, PrevClose: 100}) != true {
		t.Error("fall 3% not ≥ 2%")
	}
	if (Rule{Kind: Below, Value: 5}).Met(feed.Quote{}) {
		t.Error("no price met below")
	}
}

func TestCheckEdgeAndCooldown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.json")
	b := Open(path)
	one, err := b.Add(Rule{Symbol: "700.HK", Kind: Above, Value: 510})
	if err != nil {
		t.Fatal(err)
	}
	rep, _ := b.Add(Rule{Symbol: "700.HK", Kind: Below, Value: 500, Repeat: true})
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, feed.Shanghai)
	q := func(last float64) feed.Quote { return feed.Quote{Symbol: "700.HK", Last: last, PrevClose: 505} }
	if f := b.Check(q(511), now); len(f) != 1 || f[0].Rule.ID != one.ID {
		t.Fatalf("above: %v", f)
	}
	if f := b.Check(q(512), now); len(f) != 0 {
		t.Error("one-shot fired again")
	}
	if b.Rules()[0].Enabled {
		t.Error("one-shot still on")
	}
	if f := b.Check(q(499), now); len(f) != 1 || f[0].Rule.ID != rep.ID {
		t.Fatalf("below: %v", f)
	}
	if f := b.Check(q(498), now.Add(time.Minute)); len(f) != 0 {
		t.Error("fired while still met")
	}
	b.Check(q(501), now.Add(2*time.Minute)) // unmet: re-armed
	if f := b.Check(q(499), now.Add(3*time.Minute)); len(f) != 0 {
		t.Error("fired inside the cooldown")
	}
	b.Check(q(501), now.Add(4*time.Minute))
	if f := b.Check(q(499), now.Add(Cooldown+time.Minute)); len(f) != 1 {
		t.Error("did not fire after the cooldown")
	}
	// Other symbols, and disabled rules, are left alone.
	if f := b.Check(feed.Quote{Symbol: "AAPL.US", Last: 1}, now); len(f) != 0 {
		t.Error(f)
	}
	// Saved, and read back.
	b2 := Open(path)
	if rs := b2.Rules(); len(rs) != 2 || rs[0].Enabled || !rs[1].Repeat {
		t.Errorf("reopened %+v", rs)
	}
	if err := b2.Remove(one.ID); err != nil || len(b2.Rules()) != 1 {
		t.Error(err)
	}
	if err := b2.SetEnabled(rep.ID, false); err != nil || b2.Rules()[0].Enabled {
		t.Error(err)
	}
	if len(b2.Symbols()) != 0 {
		t.Error("disabled rules' symbols are watched")
	}
}

func TestAddValidates(t *testing.T) {
	b := Open("")
	for _, r := range []Rule{{Kind: Above, Value: 1}, {Symbol: "700.HK", Kind: Above, Value: 0}, {Symbol: "700.HK", Kind: Rise, Value: -1}} {
		if _, err := b.Add(r); err == nil {
			t.Errorf("%+v added", r)
		}
	}
	r, err := b.Add(Rule{Symbol: "700.HK", Kind: Rise, Value: 3})
	if err != nil || !r.Enabled || r.ID == "" || r.Describe() != "涨幅达到 3%" {
		t.Errorf("%+v %v %q", r, err, r.Describe())
	}
}
