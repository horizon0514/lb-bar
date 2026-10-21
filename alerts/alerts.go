// Package alerts keeps the user's price alerts, local rules checked
// against every quote, in alerts.json of the app's data folder.
package alerts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lb-bar/feed"
)

// Kind is what a rule watches.
type Kind string

const (
	Above Kind = "above" // the price at or above Value
	Below Kind = "below" // the price at or below Value
	Rise  Kind = "rise"  // up Value% or more on the day
	Fall  Kind = "fall"  // down Value% or more on the day
)

// Kinds, in the order the settings offer them.
var Kinds = []Kind{Above, Below, Rise, Fall}

// Label says a kind in Chinese.
func (k Kind) Label() string {
	switch k {
	case Above:
		return "价格高于"
	case Below:
		return "价格低于"
	case Rise:
		return "涨幅达到"
	case Fall:
		return "跌幅达到"
	}
	return string(k)
}

// Rule is one alert.
type Rule struct {
	ID      string    `json:"id"`
	Symbol  string    `json:"symbol"`
	Name    string    `json:"name,omitempty"`
	Kind    Kind      `json:"kind"`
	Value   float64   `json:"value"` // a price, or a percentage (2 for 2%)
	Enabled bool      `json:"enabled"`
	Repeat  bool      `json:"repeat,omitempty"` // fire again after it re-arms; else once, then off
	Fired   time.Time `json:"fired,omitempty"`
	// Armed: the condition was false since it last fired (or it never did).
	Disarmed bool `json:"disarmed,omitempty"`
}

// Cooldown is the least time between two firings of a repeating rule.
const Cooldown = 5 * time.Minute

// Describe says the rule: 价格高于 500.00.
func (r Rule) Describe() string {
	switch r.Kind {
	case Rise, Fall:
		return fmt.Sprintf("%s %g%%", r.Kind.Label(), r.Value)
	}
	return fmt.Sprintf("%s %s", r.Kind.Label(), trimFloat(r.Value))
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.4f", v)
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// Met tells whether q meets the rule's condition.
func (r Rule) Met(q feed.Quote) bool {
	if q.Last == 0 {
		return false
	}
	_, rate := q.Change()
	const eps = 1e-9
	switch r.Kind {
	case Above:
		return q.Last >= r.Value-eps
	case Below:
		return q.Last <= r.Value+eps
	case Rise:
		return q.PrevClose > 0 && rate*100 >= r.Value-eps
	case Fall:
		return q.PrevClose > 0 && -rate*100 >= math.Abs(r.Value)-eps
	}
	return false
}

// Fired is a rule that fired, with the quote that fired it.
type Fired struct {
	Rule  Rule
	Quote feed.Quote
	At    time.Time
}

// Book is the rules, saved to path (nothing, with no path).
type Book struct {
	path  string
	mu    sync.Mutex
	rules []Rule
}

// Open loads the rules at path; a missing or broken file is none.
func Open(path string) *Book {
	b := &Book{path: path}
	if path == "" {
		return b
	}
	if data, err := os.ReadFile(path); err == nil {
		var f struct {
			Rules []Rule `json:"rules"`
		}
		if json.Unmarshal(data, &f) == nil {
			b.rules = f.Rules
		}
	}
	return b
}

// Rules are a copy of the rules.
func (b *Book) Rules() []Rule {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Rule(nil), b.rules...)
}

// Symbols are those the enabled rules watch.
func (b *Book) Symbols() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	seen := map[string]bool{}
	for _, r := range b.rules {
		if r.Enabled && !seen[r.Symbol] {
			seen[r.Symbol] = true
			out = append(out, r.Symbol)
		}
	}
	return out
}

// Add adds a rule, enabled, and saves.
func (b *Book) Add(r Rule) (Rule, error) {
	if r.Symbol == "" {
		return r, fmt.Errorf("缺少股票代码")
	}
	switch r.Kind {
	case Above, Below:
		if r.Value <= 0 {
			return r, fmt.Errorf("价格要大于 0")
		}
	case Rise, Fall:
		if r.Value <= 0 || r.Value >= 100 {
			return r, fmt.Errorf("涨跌幅填 0 到 100 之间的数，例如 3 表示 3%%")
		}
	default:
		return r, fmt.Errorf("未知的提醒类型")
	}
	if r.ID == "" {
		var id [6]byte
		_, _ = rand.Read(id[:])
		r.ID = hex.EncodeToString(id[:])
	}
	r.Enabled = true
	b.mu.Lock()
	b.rules = append(b.rules, r)
	b.mu.Unlock()
	return r, b.save()
}

// Remove deletes rule id.
func (b *Book) Remove(id string) error {
	b.mu.Lock()
	for i, r := range b.rules {
		if r.ID == id {
			b.rules = append(b.rules[:i:i], b.rules[i+1:]...)
			break
		}
	}
	b.mu.Unlock()
	return b.save()
}

// SetEnabled turns rule id on or off; on, it is armed anew.
func (b *Book) SetEnabled(id string, on bool) error {
	b.mu.Lock()
	for i := range b.rules {
		if b.rules[i].ID == id {
			b.rules[i].Enabled = on
			if on {
				b.rules[i].Disarmed = false
			}
		}
	}
	b.mu.Unlock()
	return b.save()
}

// Check checks the enabled rules of q's symbol, and returns those that
// fire: whose condition is met and was not when they last fired. A rule
// that does not repeat turns off as it fires; one that does waits, after
// firing, for its condition to be unmet, and Cooldown.
func (b *Book) Check(q feed.Quote, now time.Time) []Fired {
	b.mu.Lock()
	var out []Fired
	changed := false
	for i := range b.rules {
		r := &b.rules[i]
		if !r.Enabled || r.Symbol != q.Symbol {
			continue
		}
		met := r.Met(q)
		switch {
		case met && !r.Disarmed && (r.Fired.IsZero() || now.Sub(r.Fired) >= Cooldown || !r.Repeat):
			r.Fired, r.Disarmed = now, true
			if !r.Repeat {
				r.Enabled = false
			}
			out = append(out, Fired{Rule: *r, Quote: q, At: now})
			changed = true
		case !met && r.Disarmed:
			r.Disarmed = false
			changed = true
		}
	}
	b.mu.Unlock()
	if changed {
		_ = b.save()
	}
	return out
}

func (b *Book) save() error {
	if b.path == "" {
		return nil
	}
	b.mu.Lock()
	data, err := json.MarshalIndent(struct {
		Rules []Rule `json:"rules"`
	}{b.rules}, "", "  ")
	b.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o700); err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}
